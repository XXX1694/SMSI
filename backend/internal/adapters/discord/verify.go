package discord

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/socialos/backend/internal/adapters/provider"
)

// Verify reads the webhook (GET returns its name and channel; the answer also
// holds the token, which is ignored) and returns a profile with ids only.
// The secret is the canonical webhook URL.
func (a *Adapter) Verify(ctx context.Context, fields map[string]string) (provider.Profile, string, error) {
	w, err := parseWebhookURL(fields["webhook_url"])
	if err != nil {
		return provider.Profile{}, "", err
	}
	req, err := http.NewRequest(http.MethodGet, a.url(w, ""), nil)
	if err != nil {
		return provider.Profile{}, "", invalidWebhook()
	}
	body, _, err := a.call(ctx, req, w)
	if err != nil {
		return provider.Profile{}, "", err
	}
	var info struct {
		ID        string `json:"id"`
		Name      string `json:"name"`
		ChannelID string `json:"channel_id"`
		GuildID   string `json:"guild_id"`
	}
	if json.Unmarshal(body, &info) != nil || info.ID != w.id {
		return provider.Profile{}, "", &provider.Error{Kind: provider.KindPermanent, Provider: Name, Code: "UNEXPECTED_ANSWER", Message: "Discord returned an unexpected answer for this webhook"}
	}
	name := info.Name
	if name == "" {
		name = "Discord webhook"
	}
	meta := map[string]any{"webhook_id": w.id}
	if info.ChannelID != "" {
		meta["channel_id"] = info.ChannelID
	}
	if info.GuildID != "" {
		meta["guild_id"] = info.GuildID
	}
	return provider.Profile{ID: w.id, Username: name, DisplayName: name, Metadata: meta}, w.canonical(), nil
}
