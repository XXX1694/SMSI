package telegram

import (
	"context"
	"errors"
	"regexp"
	"strconv"
	"strings"

	"github.com/socialos/backend/internal/adapters/provider"
)

type tgUser struct {
	ID       int64  `json:"id"`
	IsBot    bool   `json:"is_bot"`
	Username string `json:"username"`
}

type tgChat struct {
	ID       int64  `json:"id"`
	Type     string `json:"type"`
	Title    string `json:"title"`
	Username string `json:"username"`
}

type tgMember struct {
	Status          string `json:"status"`
	CanPostMessages *bool  `json:"can_post_messages"`
}

var (
	usernameRe = regexp.MustCompile(`^@?([A-Za-z][A-Za-z0-9_]{3,31})$`)
	chatIDRe   = regexp.MustCompile(`^-?\d{1,20}$`)
)

// NormalizeChat accepts @name, name, https://t.me/name or a numeric id.
func NormalizeChat(raw string) (string, bool) {
	s := strings.TrimSpace(raw)
	for _, p := range []string{"https://t.me/", "http://t.me/", "t.me/"} {
		s = strings.TrimPrefix(s, p)
	}
	s = strings.TrimSuffix(s, "/")
	if chatIDRe.MatchString(s) {
		return s, true
	}
	if m := usernameRe.FindStringSubmatch(s); m != nil {
		return "@" + m[1], true
	}
	return "", false
}

func invalidChat(msg string) *provider.Error {
	return &provider.Error{Kind: provider.KindPermanent, Provider: Name, Code: "CHAT_INVALID", Message: msg}
}

// VerifyChat checks that the configured bot can post to the chat.
func (a *Adapter) VerifyChat(ctx context.Context, chat string) (provider.Profile, error) {
	chatID, ok := NormalizeChat(chat)
	if !ok {
		return provider.Profile{}, invalidChat("chat must be @username, t.me link or numeric chat id")
	}
	var me tgUser
	if err := a.callJSON(ctx, "getMe", map[string]any{}, &me); err != nil {
		return provider.Profile{}, err
	}
	var c tgChat
	if err := a.callJSON(ctx, "getChat", map[string]any{"chat_id": chatID}, &c); err != nil {
		var pe *provider.Error
		if errors.As(err, &pe) && pe.Kind == provider.KindPermanent && pe.Code != "BOT_UNAUTHORIZED" {
			return provider.Profile{}, invalidChat("chat not found or the bot has no access to it")
		}
		return provider.Profile{}, err
	}
	if c.Type == "private" {
		return provider.Profile{}, invalidChat("only channels and groups can be connected")
	}
	var m tgMember
	if err := a.callJSON(ctx, "getChatMember", map[string]any{"chat_id": c.ID, "user_id": me.ID}, &m); err != nil {
		return provider.Profile{}, err
	}
	if err := checkRights(c.Type, m); err != nil {
		return provider.Profile{}, err
	}
	return provider.Profile{
		ID:          strconv.FormatInt(c.ID, 10),
		Username:    c.Username,
		DisplayName: c.Title,
		Metadata: map[string]any{
			"chat_id": strconv.FormatInt(c.ID, 10), "chat_type": c.Type,
			"bot_id": me.ID, "bot_username": me.Username, "token_ref": TokenRef,
		},
	}, nil
}

func checkRights(chatType string, m tgMember) error {
	switch m.Status {
	case "creator":
		return nil
	case "administrator":
		if chatType == "channel" && m.CanPostMessages != nil && !*m.CanPostMessages {
			return invalidChat("the bot is an admin but lacks the 'Post messages' right")
		}
		return nil
	case "member":
		if chatType != "channel" {
			return nil
		}
	}
	return invalidChat("add the bot as an administrator of the channel first")
}
