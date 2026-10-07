package telegram

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"sync"
	"time"

	"github.com/socialos/backend/internal/adapters/provider"
)

// AllowedUpdates are the update kinds requested from Telegram (getUpdates and
// setWebhook). my_chat_member is delivered but currently ignored; it is part of
// the subscription so removal/demotion of the bot can be handled later without
// re-registering the webhook.
var AllowedUpdates = []string{"channel_post", "message", "my_chat_member"}

// maxUpdateBatch bounds one getUpdates response so a flood of large messages in
// a busy group cannot exceed the response size cap.
const maxUpdateBatch = 25

// botUsernameTTL is how long the bot's @username is cached.
const botUsernameTTL = time.Hour

// update is the subset of a Bot API Update this adapter understands.
type update struct {
	UpdateID    int64      `json:"update_id"`
	ChannelPost *inMessage `json:"channel_post"`
	Message     *inMessage `json:"message"`
	// Everything else (edited_*, my_chat_member, callback_query, ...) is irrelevant.
}

type inMessage struct {
	MessageID  int64   `json:"message_id"`
	From       *tgUser `json:"from"`
	SenderChat *tgChat `json:"sender_chat"`
	Chat       tgChat  `json:"chat"`
	Text       string  `json:"text"`
}

// UpdateID extracts update_id from one raw update (used to advance the offset).
func UpdateID(raw []byte) (int64, error) {
	var u struct {
		UpdateID *int64 `json:"update_id"`
	}
	if err := json.Unmarshal(raw, &u); err != nil || u.UpdateID == nil {
		return 0, errors.New("telegram: update without update_id")
	}
	return *u.UpdateID, nil
}

// ParseUpdate extracts the text message that may carry a link code.
//
// Only two shapes are relevant: a channel post (only channel admins can post
// there, so the poster is trusted) and a text message in a group or supergroup.
// For groups anyone can write, so the sender is returned for an admin check;
// the exception is an anonymous admin (sender_chat is the group itself), whom
// Telegram also restricts to admins. Messages sent on behalf of another chat
// (a channel posting into its discussion group), edits, private chats, bots
// and every other update kind are ignored: (nil, nil).
func (a *Adapter) ParseUpdate(raw []byte) (*provider.ChatMessage, error) {
	var u update
	if err := json.Unmarshal(raw, &u); err != nil {
		return nil, fmt.Errorf("telegram: malformed update: %w", err)
	}
	switch {
	case u.ChannelPost != nil:
		m := u.ChannelPost
		if m.Chat.Type != "channel" || m.Text == "" {
			return nil, nil
		}
		return &provider.ChatMessage{ChatID: strconv.FormatInt(m.Chat.ID, 10), MessageID: m.MessageID, Text: m.Text, SenderTrusted: true}, nil
	case u.Message != nil:
		m := u.Message
		if (m.Chat.Type != "group" && m.Chat.Type != "supergroup") || m.Text == "" {
			return nil, nil
		}
		msg := &provider.ChatMessage{ChatID: strconv.FormatInt(m.Chat.ID, 10), MessageID: m.MessageID, Text: m.Text}
		switch {
		case m.SenderChat != nil && m.SenderChat.ID == m.Chat.ID:
			msg.SenderTrusted = true // anonymous group admin
		case m.SenderChat != nil:
			return nil, nil // posted on behalf of some other chat
		case m.From == nil || m.From.IsBot:
			return nil, nil
		default:
			msg.SenderID = strconv.FormatInt(m.From.ID, 10)
		}
		return msg, nil
	}
	return nil, nil
}

// GetUpdates long-polls the Bot API. offset confirms every update below it.
func (a *Adapter) GetUpdates(ctx context.Context, offset int64, timeout time.Duration) ([]json.RawMessage, error) {
	params := map[string]any{
		"offset": offset, "timeout": int(timeout / time.Second), "limit": maxUpdateBatch, "allowed_updates": AllowedUpdates,
	}
	var out []json.RawMessage
	if err := a.callJSONLimit(ctx, "getUpdates", params, &out, maxUpdatesRespBytes); err != nil {
		return nil, err
	}
	return out, nil
}

type botIdentity struct {
	mu      sync.Mutex
	name    string
	fetched time.Time
}

// BotUsername returns the bot's public @handle (without the @), cached for an hour.
func (a *Adapter) BotUsername(ctx context.Context) (string, error) {
	a.bot.mu.Lock()
	defer a.bot.mu.Unlock()
	if a.bot.name != "" && time.Since(a.bot.fetched) < botUsernameTTL {
		return a.bot.name, nil
	}
	var me tgUser
	if err := a.callJSON(ctx, "getMe", map[string]any{}, &me); err != nil {
		return "", err
	}
	if me.Username == "" {
		return "", &provider.Error{Kind: provider.KindPermanent, Provider: Name, Code: "BOT_NO_USERNAME", Message: "the Telegram bot has no username"}
	}
	a.bot.name, a.bot.fetched = me.Username, time.Now()
	return a.bot.name, nil
}

// IsChatAdmin reports whether the user is the owner or an administrator of the chat.
func (a *Adapter) IsChatAdmin(ctx context.Context, chatID, userID string) (bool, error) {
	cid, err := strconv.ParseInt(chatID, 10, 64)
	if err != nil {
		return false, invalidChat("invalid chat id")
	}
	uid, err := strconv.ParseInt(userID, 10, 64)
	if err != nil {
		return false, invalidChat("invalid user id")
	}
	var m tgMember
	if err := a.callJSON(ctx, "getChatMember", map[string]any{"chat_id": cid, "user_id": uid}, &m); err != nil {
		return false, err
	}
	return m.Status == "creator" || m.Status == "administrator", nil
}

// DeleteMessage removes one message from a chat (the bot needs the right to delete messages).
func (a *Adapter) DeleteMessage(ctx context.Context, chatID string, messageID int64) error {
	cid, err := strconv.ParseInt(chatID, 10, 64)
	if err != nil {
		return invalidChat("invalid chat id")
	}
	return a.callJSON(ctx, "deleteMessage", map[string]any{"chat_id": cid, "message_id": messageID}, nil)
}

// LinkInstructions is the user-facing text shown next to a link code.
func (a *Adapter) LinkInstructions(botUsername, code string, ttl time.Duration) string {
	return fmt.Sprintf("1. Add @%s as an administrator of your Telegram channel or group with the \"Post messages\" right. "+
		"2. Post this code there as a normal message: %s. "+
		"The code expires in %d minutes and works once; the bot deletes the message after linking.",
		botUsername, code, int(ttl/time.Minute))
}
