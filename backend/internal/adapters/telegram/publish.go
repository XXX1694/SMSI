package telegram

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/socialos/backend/internal/adapters/provider"
)

type tgMessage struct {
	MessageID int64 `json:"message_id"`
}

func chatIDOf(acc provider.AccountRef) string {
	if v, ok := acc.Metadata["chat_id"].(string); ok && v != "" {
		return v
	}
	return acc.ProviderAccountID
}

func validate(req provider.PublishRequest) error {
	n := utf8.RuneCountInString(req.Text)
	switch {
	case len(req.Media) == 0 && n == 0:
		return invalid("TEXT_EMPTY", "Telegram message cannot be empty")
	case len(req.Media) == 0 && n > MaxTextLength:
		return invalid("TEXT_TOO_LONG", fmt.Sprintf("Telegram messages are limited to %d characters", MaxTextLength))
	case len(req.Media) > 0 && n > MaxCaptionLength:
		return invalid("CAPTION_TOO_LONG", fmt.Sprintf("Telegram captions are limited to %d characters", MaxCaptionLength))
	case len(req.Media) > MaxMedia:
		return invalid("TOO_MANY_MEDIA", fmt.Sprintf("Telegram albums are limited to %d items", MaxMedia))
	}
	for _, m := range req.Media {
		if m.Kind == "image" && m.Size > MaxPhotoBytes {
			return invalid("MEDIA_TOO_LARGE", "Telegram photos are limited to 10 MB")
		}
		if m.Size > MaxUploadBytes {
			return invalid("MEDIA_TOO_LARGE", "Telegram bot uploads are limited to 50 MB")
		}
	}
	return nil
}

func invalid(code, msg string) *provider.Error {
	return &provider.Error{Kind: provider.KindPermanent, Provider: Name, Code: code, Message: msg}
}

// Publish sends a text message, a single photo/video, or an album.
func (a *Adapter) Publish(ctx context.Context, req provider.PublishRequest) (provider.PublishResult, error) {
	if err := validate(req); err != nil {
		return provider.PublishResult{}, err
	}
	chatID := chatIDOf(req.Account)
	var ids []int64
	var err error
	switch len(req.Media) {
	case 0:
		var m tgMessage
		err = a.callJSON(ctx, "sendMessage", map[string]any{"chat_id": chatID, "text": req.Text}, &m)
		ids = []int64{m.MessageID}
	case 1:
		var m tgMessage
		err = a.sendSingle(ctx, chatID, req.Text, req.Media[0], &m)
		ids = []int64{m.MessageID}
	default:
		ids, err = a.sendAlbum(ctx, chatID, req.Text, req.Media)
	}
	if err != nil {
		return provider.PublishResult{}, err
	}
	return provider.PublishResult{
		ExternalID: externalID(chatID, ids),
		URL:        messageURL(req.Account.Username, chatID, ids[0]),
		Metadata:   map[string]any{"message_ids": ids, "chat_id": chatID},
	}, nil
}

func externalID(chatID string, ids []int64) string {
	parts := make([]string, len(ids))
	for i, id := range ids {
		parts[i] = strconv.FormatInt(id, 10)
	}
	return chatID + ":" + strings.Join(parts, ",")
}

func parseExternalID(ext string) (string, []int64, error) {
	i := strings.LastIndex(ext, ":")
	if i <= 0 {
		return "", nil, invalid("BAD_EXTERNAL_ID", "malformed Telegram post id")
	}
	var ids []int64
	for _, p := range strings.Split(ext[i+1:], ",") {
		n, err := strconv.ParseInt(p, 10, 64)
		if err != nil {
			return "", nil, invalid("BAD_EXTERNAL_ID", "malformed Telegram post id")
		}
		ids = append(ids, n)
	}
	return ext[:i], ids, nil
}

// messageURL builds a public link: t.me/<user>/<id> or t.me/c/<internal>/<id>.
func messageURL(username, chatID string, msgID int64) string {
	if username != "" {
		return fmt.Sprintf("https://t.me/%s/%d", strings.TrimPrefix(username, "@"), msgID)
	}
	if strings.HasPrefix(chatID, "-100") {
		return fmt.Sprintf("https://t.me/c/%s/%d", strings.TrimPrefix(chatID, "-100"), msgID)
	}
	return ""
}

// Delete removes the message(s) of a published post.
func (a *Adapter) Delete(ctx context.Context, d provider.DeleteRequest) error {
	chatID, ids, err := parseExternalID(d.ExternalID)
	if err != nil {
		return err
	}
	if len(ids) == 1 {
		return a.callJSON(ctx, "deleteMessage", map[string]any{"chat_id": chatID, "message_id": ids[0]}, nil)
	}
	return a.callJSON(ctx, "deleteMessages", map[string]any{"chat_id": chatID, "message_ids": ids}, nil)
}
