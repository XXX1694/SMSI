package linkedin

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"

	"github.com/socialos/backend/internal/adapters/provider"
)

type initUploadResponse struct {
	Value struct {
		UploadURL string `json:"uploadUrl"`
		Image     string `json:"image"`
	} `json:"value"`
}

// uploadImage registers an image and uploads its bytes; returns urn:li:image:….
func (a *Adapter) uploadImage(ctx context.Context, token, owner string, m provider.MediaFile) (string, error) {
	payload, _ := json.Marshal(map[string]any{"initializeUploadRequest": map[string]any{"owner": owner}})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, a.cfg.APIBaseURL+"/rest/images?action=initializeUpload", bytes.NewReader(payload))
	if err != nil {
		return "", err
	}
	a.setRESTHeaders(req, token)
	resp, err := a.http.Do(req)
	if err != nil {
		return "", err
	}
	defer drain(resp)
	if resp.StatusCode != http.StatusOK {
		return "", errorFromResponse(resp)
	}
	var ir initUploadResponse
	if err := json.NewDecoder(limited(resp)).Decode(&ir); err != nil || ir.Value.UploadURL == "" || ir.Value.Image == "" {
		return "", permanent("BAD_UPLOAD_RESPONSE", "invalid initializeUpload response")
	}
	if err := a.putBinary(ctx, ir.Value.UploadURL, token, m); err != nil {
		return "", err
	}
	return ir.Value.Image, nil
}

func (a *Adapter) putBinary(ctx context.Context, uploadURL, token string, m provider.MediaFile) error {
	body, err := m.Open(ctx)
	if err != nil {
		return &provider.Error{Kind: provider.KindRetryable, Provider: Name, Code: "MEDIA_READ", Message: "could not read media from storage", Err: err}
	}
	defer func() { _ = body.Close() }()
	req, err := http.NewRequestWithContext(ctx, http.MethodPut, uploadURL, body)
	if err != nil {
		return err
	}
	if m.Size > 0 {
		req.ContentLength = m.Size
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", m.MimeType)
	resp, err := a.http.Do(req)
	if err != nil {
		return err
	}
	defer drain(resp)
	if resp.StatusCode/100 != 2 {
		return errorFromResponse(resp)
	}
	return nil
}
