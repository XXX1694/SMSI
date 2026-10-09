package discord

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/textproto"
	"path"
	"strings"
	"unicode/utf8"

	"github.com/socialos/backend/internal/adapters/provider"
)

type attachment struct {
	ID       int    `json:"id"`
	Filename string `json:"filename"`
}

// payload never pings: allowed_mentions.parse is empty, so "@everyone" in a post stays plain text.
type payload struct {
	Content         string              `json:"content,omitempty"`
	AllowedMentions map[string][]string `json:"allowed_mentions"`
	Attachments     []attachment        `json:"attachments,omitempty"`
}

func newPayload(text string) payload {
	return payload{Content: text, AllowedMentions: map[string][]string{"parse": {}}}
}

func permanent(code, msg string) *provider.Error {
	return &provider.Error{Kind: provider.KindPermanent, Provider: Name, Code: code, Message: msg}
}

// Publish posts text and images with ?wait=true so Discord returns the message id.
func (a *Adapter) Publish(ctx context.Context, req provider.PublishRequest) (provider.PublishResult, error) {
	w, err := parseWebhookURL(req.AccessToken)
	if err != nil {
		return provider.PublishResult{}, &provider.Error{Kind: provider.KindAuth, Provider: Name, Code: "NO_CREDENTIALS", Message: "stored Discord credentials are unusable; reconnect the channel"}
	}
	if err := checkContent(req); err != nil {
		return provider.PublishResult{}, err
	}
	hreq, cleanup, err := a.publishRequest(ctx, w, req)
	if err != nil {
		return provider.PublishResult{}, err
	}
	defer cleanup()
	body, _, err := a.call(ctx, hreq, w)
	if err != nil {
		return provider.PublishResult{}, err
	}
	var msg struct {
		ID        string `json:"id"`
		ChannelID string `json:"channel_id"`
	}
	if json.Unmarshal(body, &msg) != nil || msg.ID == "" {
		// Discord accepted the request but we cannot tell which message it is.
		return provider.PublishResult{}, &provider.Error{Kind: provider.KindUnknown, Provider: Name, Code: "EMPTY_RESULT", Message: "Discord did not return the message id"}
	}
	return provider.PublishResult{ExternalID: msg.ID, URL: messageURL(req.Account.Metadata, msg.ChannelID, msg.ID)}, nil
}

func checkContent(req provider.PublishRequest) error {
	if utf8.RuneCountInString(req.Text) > MaxTextLength {
		return permanent("TEXT_TOO_LONG", fmt.Sprintf("Discord allows at most %d characters", MaxTextLength))
	}
	if strings.TrimSpace(req.Text) == "" && len(req.Media) == 0 {
		return permanent("EMPTY_POST", "Discord needs text or an image")
	}
	if len(req.Media) > MaxMedia {
		return permanent("TOO_MANY_FILES", fmt.Sprintf("Discord allows at most %d images per post", MaxMedia))
	}
	for _, m := range req.Media {
		if m.Kind != "image" {
			return fmt.Errorf("discord %s attachments: %w", m.Kind, provider.ErrUnsupported)
		}
		if m.Size > MaxFileBytes {
			return permanent("FILE_TOO_LARGE", fmt.Sprintf("Discord images must be at most %d MB", MaxFileBytes>>20))
		}
	}
	return nil
}

// publishRequest builds JSON for text-only posts and a streamed multipart body
// (payload_json + files[n]) when there are images.
func (a *Adapter) publishRequest(ctx context.Context, w webhook, req provider.PublishRequest) (*http.Request, func(), error) {
	target := a.url(w, "?wait=true")
	p := newPayload(req.Text)
	if len(req.Media) == 0 {
		b, _ := json.Marshal(p)
		r, err := http.NewRequest(http.MethodPost, target, bytes.NewReader(b))
		if err != nil {
			return nil, nil, permanent("BAD_REQUEST", "could not build the request")
		}
		r.Header.Set("Content-Type", "application/json")
		return r, func() {}, nil
	}
	for i, m := range req.Media {
		p.Attachments = append(p.Attachments, attachment{ID: i, Filename: fileName(m, i)})
	}
	pr, pw := io.Pipe()
	mw := multipart.NewWriter(pw)
	go func() { _ = pw.CloseWithError(writeParts(ctx, mw, p, req.Media)) }()
	r, err := http.NewRequest(http.MethodPost, target, pr)
	if err != nil {
		_ = pr.Close()
		return nil, nil, permanent("BAD_REQUEST", "could not build the request")
	}
	r.Header.Set("Content-Type", mw.FormDataContentType())
	return r, func() { _ = pr.Close() }, nil
}

func writeParts(ctx context.Context, mw *multipart.Writer, p payload, media []provider.MediaFile) error {
	b, _ := json.Marshal(p)
	if err := mw.WriteField("payload_json", string(b)); err != nil {
		return err
	}
	for i, m := range media {
		if err := writeFile(ctx, mw, i, p.Attachments[i].Filename, m); err != nil {
			return err
		}
	}
	return mw.Close()
}

func writeFile(ctx context.Context, mw *multipart.Writer, i int, name string, m provider.MediaFile) error {
	rc, err := m.Open(ctx)
	if err != nil {
		return fmt.Errorf("open media: %w", err)
	}
	defer func() { _ = rc.Close() }()
	h := textproto.MIMEHeader{}
	h.Set("Content-Disposition", fmt.Sprintf(`form-data; name="files[%d]"; filename=%q`, i, name))
	h.Set("Content-Type", m.MimeType)
	part, err := mw.CreatePart(h)
	if err != nil {
		return err
	}
	_, err = io.Copy(part, rc)
	return err
}

// fileName is a safe base name; Discord needs unique names per message.
func fileName(m provider.MediaFile, i int) string {
	n := path.Base(strings.ReplaceAll(m.Name, `\`, "/"))
	if n == "." || n == "/" || n == "" {
		n = "image"
	}
	return fmt.Sprintf("%d-%s", i+1, n)
}

func messageURL(meta map[string]any, channelFromMessage, messageID string) string {
	guild, _ := meta["guild_id"].(string)
	channel := channelFromMessage
	if channel == "" {
		channel, _ = meta["channel_id"].(string)
	}
	if guild == "" || channel == "" {
		return ""
	}
	return "https://discord.com/channels/" + guild + "/" + channel + "/" + messageID
}

// Delete removes a message the webhook created. A message that is already gone
// counts as deleted.
func (a *Adapter) Delete(ctx context.Context, req provider.DeleteRequest) error {
	w, err := parseWebhookURL(req.AccessToken)
	if err != nil {
		return &provider.Error{Kind: provider.KindAuth, Provider: Name, Code: "NO_CREDENTIALS", Message: "stored Discord credentials are unusable; reconnect the channel"}
	}
	if !idPattern.MatchString(req.ExternalID) {
		return permanent("BAD_MESSAGE_ID", "not a Discord message id")
	}
	hreq, err := http.NewRequest(http.MethodDelete, a.url(w, "/messages/"+req.ExternalID), nil)
	if err != nil {
		return permanent("BAD_REQUEST", "could not build the request")
	}
	_, _, err = a.call(ctx, hreq, w)
	var pe *provider.Error
	if errors.As(err, &pe) && pe.HTTPStatus == http.StatusNotFound && pe.Code == "DISCORD_"+fmt.Sprint(codeUnknownMessage) {
		return nil
	}
	return err
}
