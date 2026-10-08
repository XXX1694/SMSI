package mastodon

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/socialos/backend/internal/adapters/provider"
)

// maxUploadBytes bounds what is buffered for one upload. The composer already
// enforces the instance's image limit; this is the adapter's own backstop.
const maxUploadBytes = 64 << 20

// Visibility of published statuses. Unlisted could be exposed later as an option.
const visibility = "public"

type mediaAttachment struct {
	ID string `json:"id"`
}

type status struct {
	ID  string `json:"id"`
	URL string `json:"url"`
	URI string `json:"uri"`
}

// Publish uploads the images, then creates the status.
//
// Idempotency: Mastodon remembers the Idempotency-Key of a created status for
// about an hour and answers a repeat with the same status instead of a new
// one. The key is derived from req.IdempotencyKey, the stable id of the post
// target, so every attempt of the same target sends the same key. A timeout
// after the request left (outcome Unknown) can therefore be re-sent, which is
// why Capabilities declares SafeToRetryAfterUnknown. An image upload retried
// after a timeout may leave an orphaned unattached upload on the server; the
// server removes those on its own.
func (a *Adapter) Publish(ctx context.Context, req provider.PublishRequest) (provider.PublishResult, error) {
	base, err := accountBase(req.Account)
	if err != nil {
		return provider.PublishResult{}, err
	}
	if strings.TrimSpace(req.IdempotencyKey) == "" {
		return provider.PublishResult{}, &provider.Error{Kind: provider.KindPermanent, Provider: Name, Code: "IDEMPOTENCY_KEY_REQUIRED", Message: "Mastodon: a publish needs an idempotency key"}
	}
	ids := make([]string, 0, len(req.Media))
	for _, m := range req.Media {
		id, err := a.uploadMedia(ctx, base, req.AccessToken, m)
		if err != nil {
			return provider.PublishResult{}, err
		}
		ids = append(ids, id)
	}
	payload := map[string]any{"status": req.Text, "visibility": visibility}
	if len(ids) > 0 {
		payload["media_ids"] = ids
	}
	r, err := a.postJSON(ctx, base, "/api/v1/statuses", req.AccessToken, payload, "socialos-"+req.IdempotencyKey)
	if err != nil {
		return provider.PublishResult{}, err
	}
	if r.status != http.StatusOK {
		return provider.PublishResult{}, a.failure(r, req.AccessToken)
	}
	var st status
	if err := json.Unmarshal(r.body, &st); err != nil || st.ID == "" {
		// The server accepted the request but we cannot say which status it made.
		return provider.PublishResult{}, &provider.Error{Kind: provider.KindUnknown, Provider: Name, Code: "BAD_RESPONSE", Message: "Mastodon: the server accepted the post but returned no id"}
	}
	link := st.URL
	if link == "" {
		link = st.URI
	}
	if !strings.HasPrefix(link, "https://") {
		link = ""
	}
	return provider.PublishResult{ExternalID: st.ID, URL: link, Metadata: map[string]any{"visibility": visibility}}, nil
}

// Delete removes a status. A status that is already gone counts as deleted.
func (a *Adapter) Delete(ctx context.Context, req provider.DeleteRequest) error {
	base, err := accountBase(req.Account)
	if err != nil {
		return err
	}
	if req.ExternalID == "" {
		return &provider.Error{Kind: provider.KindPermanent, Provider: Name, Code: "ID_REQUIRED", Message: "Mastodon: the post id is missing"}
	}
	hr, err := http.NewRequestWithContext(ctx, http.MethodDelete, base+"/api/v1/statuses/"+url.PathEscape(req.ExternalID), nil)
	if err != nil {
		return err
	}
	r, err := a.call(hr, req.AccessToken)
	if err != nil {
		return err
	}
	if r.status == http.StatusOK || r.status == http.StatusNotFound || r.status == http.StatusGone {
		return nil
	}
	return a.failure(r, req.AccessToken)
}

// accountBase rebuilds the instance origin from the stored, non-secret host.
func accountBase(acc provider.AccountRef) (string, error) {
	host, _ := acc.Metadata["host"].(string)
	if host == "" {
		if i := strings.LastIndex(acc.ProviderAccountID, ":"); i > 0 {
			host = acc.ProviderAccountID[:i]
		}
	}
	base, _, err := instanceBase("https://" + host)
	if err != nil {
		return "", &provider.Error{Kind: provider.KindAuth, Provider: Name, Code: "ACCOUNT_INCOMPLETE", Message: "Mastodon: the account has no instance address, reconnect it"}
	}
	return base, nil
}

// uploadMedia sends one image to /api/v2/media (v1 on servers without v2) and
// waits until the server finished processing it.
func (a *Adapter) uploadMedia(ctx context.Context, base, token string, m provider.MediaFile) (string, error) {
	if m.Kind != "image" {
		return "", &provider.Error{Kind: provider.KindPermanent, Provider: Name, Code: "UNSUPPORTED_MEDIA", Message: "Mastodon: only images are supported"}
	}
	if m.Size > maxUploadBytes {
		return "", &provider.Error{Kind: provider.KindPermanent, Provider: Name, Code: "MEDIA_TOO_LARGE", Message: "Mastodon: the image is too large"}
	}
	rc, err := m.Open(ctx)
	if err != nil {
		return "", fmt.Errorf("open media: %w", err)
	}
	defer func() { _ = rc.Close() }()
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	name := m.Name
	if name == "" {
		name = "image"
	}
	part, err := mw.CreateFormFile("file", name)
	if err != nil {
		return "", err
	}
	if _, err := io.Copy(part, io.LimitReader(rc, maxUploadBytes+1)); err != nil {
		return "", fmt.Errorf("read media: %w", err)
	}
	if err := mw.Close(); err != nil {
		return "", err
	}
	if buf.Len() > maxUploadBytes {
		return "", &provider.Error{Kind: provider.KindPermanent, Provider: Name, Code: "MEDIA_TOO_LARGE", Message: "Mastodon: the image is too large"}
	}
	r, err := a.postMedia(ctx, base, "/api/v2/media", token, mw.FormDataContentType(), buf.Bytes())
	if err == nil && r.status == http.StatusNotFound {
		r, err = a.postMedia(ctx, base, "/api/v1/media", token, mw.FormDataContentType(), buf.Bytes())
	}
	if err != nil {
		return "", err
	}
	var att mediaAttachment
	switch r.status {
	case http.StatusOK:
		if json.Unmarshal(r.body, &att) != nil || att.ID == "" {
			return "", badMedia()
		}
		return att.ID, nil
	case http.StatusAccepted:
		if json.Unmarshal(r.body, &att) != nil || att.ID == "" {
			return "", badMedia()
		}
		return att.ID, a.waitForMedia(ctx, base, token, att.ID)
	default:
		return "", a.failure(r, token)
	}
}

func badMedia() error {
	return &provider.Error{Kind: provider.KindPermanent, Provider: Name, Code: "BAD_RESPONSE", Message: "Mastodon: the server returned no id for the uploaded image"}
}

func (a *Adapter) postMedia(ctx context.Context, base, path, token, contentType string, body []byte) (reply, error) {
	hr, err := http.NewRequestWithContext(ctx, http.MethodPost, base+path, bytes.NewReader(body))
	if err != nil {
		return reply{}, err
	}
	hr.Header.Set("Content-Type", contentType)
	return a.call(hr, token)
}

// waitForMedia polls GET /api/v1/media/:id. The server answers 206 while it is
// still processing and 200 when the file is ready. It stops when ctx ends or
// after MediaProcessingTimeout; nothing was published then, so that is Retryable.
func (a *Adapter) waitForMedia(ctx context.Context, base, token, id string) error {
	deadline := time.NewTimer(a.cfg.MediaProcessingTimeout)
	defer deadline.Stop()
	tick := time.NewTicker(a.cfg.MediaPollInterval)
	defer tick.Stop()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-deadline.C:
			return &provider.Error{Kind: provider.KindRetryable, Provider: Name, Code: "MEDIA_PROCESSING_TIMEOUT", Message: "Mastodon: the server is still processing the image, try again later"}
		case <-tick.C:
		}
		r, err := a.get(ctx, base, "/api/v1/media/"+url.PathEscape(id), token)
		if err != nil {
			return err
		}
		switch r.status {
		case http.StatusOK:
			return nil
		case http.StatusPartialContent:
		default:
			return a.failure(r, token)
		}
	}
}
