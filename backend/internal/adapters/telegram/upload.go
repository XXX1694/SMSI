package telegram

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/textproto"

	"github.com/socialos/backend/internal/adapters/provider"
)

type field struct{ name, value string }

type filePart struct {
	field string
	media provider.MediaFile
}

// sendSingle uploads one photo/video/animation with an optional caption.
func (a *Adapter) sendSingle(ctx context.Context, chatID, caption string, m provider.MediaFile, out *tgMessage) error {
	method, fieldName := "sendPhoto", "photo"
	switch {
	case m.Kind == "video":
		method, fieldName = "sendVideo", "video"
	case m.MimeType == "image/gif":
		method, fieldName = "sendAnimation", "animation"
	}
	fields := []field{{"chat_id", chatID}}
	if caption != "" {
		fields = append(fields, field{"caption", caption})
	}
	if m.Kind == "video" {
		fields = append(fields, field{"supports_streaming", "true"})
	}
	return a.callMultipart(ctx, method, fields, []filePart{{fieldName, m}}, out)
}

type inputMedia struct {
	Type    string `json:"type"`
	Media   string `json:"media"`
	Caption string `json:"caption,omitempty"`
}

// sendAlbum uploads 2-10 items with sendMediaGroup; caption goes on the first item.
func (a *Adapter) sendAlbum(ctx context.Context, chatID, caption string, media []provider.MediaFile) ([]int64, error) {
	items := make([]inputMedia, len(media))
	parts := make([]filePart, len(media))
	for i, m := range media {
		typ := "photo"
		if m.Kind == "video" {
			typ = "video"
		}
		name := fmt.Sprintf("file%d", i)
		items[i] = inputMedia{Type: typ, Media: "attach://" + name}
		parts[i] = filePart{name, m}
	}
	items[0].Caption = caption
	spec, _ := json.Marshal(items)
	var msgs []tgMessage
	if err := a.callMultipart(ctx, "sendMediaGroup", []field{{"chat_id", chatID}, {"media", string(spec)}}, parts, &msgs); err != nil {
		return nil, err
	}
	if len(msgs) == 0 {
		return nil, &provider.Error{Kind: provider.KindUnknown, Provider: Name, Code: "EMPTY_RESULT", Message: "Telegram returned no messages"}
	}
	ids := make([]int64, len(msgs))
	for i, m := range msgs {
		ids[i] = m.MessageID
	}
	return ids, nil
}

// callMultipart streams files from storage straight into the request body.
func (a *Adapter) callMultipart(ctx context.Context, method string, fields []field, files []filePart, out any) error {
	pr, pw := io.Pipe()
	mw := multipart.NewWriter(pw)
	go func() { pw.CloseWithError(writeMultipart(ctx, mw, fields, files)) }()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, a.methodURL(method), pr)
	if err != nil {
		_ = pr.Close()
		return a.redact(err)
	}
	req.Header.Set("Content-Type", mw.FormDataContentType())
	err = a.do(req, out)
	_ = pr.Close()
	return err
}

func writeMultipart(ctx context.Context, mw *multipart.Writer, fields []field, files []filePart) error {
	for _, f := range fields {
		if err := mw.WriteField(f.name, f.value); err != nil {
			return err
		}
	}
	for _, f := range files {
		if err := writeFile(ctx, mw, f); err != nil {
			return err
		}
	}
	return mw.Close()
}

func writeFile(ctx context.Context, mw *multipart.Writer, f filePart) error {
	rc, err := f.media.Open(ctx)
	if err != nil {
		return fmt.Errorf("open media: %w", err)
	}
	defer func() { _ = rc.Close() }()
	name := f.media.Name
	if name == "" {
		name = f.field
	}
	h := textproto.MIMEHeader{}
	h.Set("Content-Disposition", fmt.Sprintf(`form-data; name=%q; filename=%q`, f.field, name))
	h.Set("Content-Type", f.media.MimeType)
	w, err := mw.CreatePart(h)
	if err != nil {
		return err
	}
	_, err = io.Copy(w, rc)
	return err
}
