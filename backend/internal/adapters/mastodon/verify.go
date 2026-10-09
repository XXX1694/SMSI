package mastodon

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"

	"github.com/socialos/backend/internal/adapters/provider"
)

type credentials struct {
	ID          string `json:"id"`
	Username    string `json:"username"`
	DisplayName string `json:"display_name"`
	Avatar      string `json:"avatar"`
}

// instanceInfo is the part of /api/v2/instance (or /api/v1/instance) that holds limits.
type instanceInfo struct {
	Configuration struct {
		Statuses struct {
			MaxCharacters int `json:"max_characters"`
			MaxMedia      int `json:"max_media_attachments"`
		} `json:"statuses"`
		Media struct {
			ImageSizeLimit int64 `json:"image_size_limit"`
		} `json:"media_attachments"`
	} `json:"configuration"`
}

// Verify checks the token with a live whoami call and reads the instance limits.
// provider_account_id is host:id, so the same person on two servers stays two accounts.
func (a *Adapter) Verify(ctx context.Context, fields map[string]string) (provider.Profile, string, error) {
	token := strings.TrimSpace(fields["access_token"])
	if token == "" {
		return provider.Profile{}, "", &provider.Error{Kind: provider.KindPermanent, Provider: Name, Code: "TOKEN_REQUIRED", Message: "Mastodon: the access token is required"}
	}
	base, host, err := instanceBase(fields["instance_url"])
	if err != nil {
		return provider.Profile{}, "", err
	}
	r, err := a.get(ctx, base, "/api/v1/accounts/verify_credentials", token)
	if err != nil {
		return provider.Profile{}, "", err
	}
	if r.status != http.StatusOK {
		return provider.Profile{}, "", a.failure(r, token)
	}
	var me credentials
	if err := json.Unmarshal(r.body, &me); err != nil || me.ID == "" || me.Username == "" {
		return provider.Profile{}, "", &provider.Error{Kind: provider.KindPermanent, Provider: Name, Code: "NOT_MASTODON",
			Message: "Mastodon: this address did not answer like a Mastodon-compatible server"}
	}
	limits, err := a.limits(ctx, base, token)
	if err != nil {
		return provider.Profile{}, "", err
	}
	handle := "@" + me.Username + "@" + host
	meta := map[string]any{"host": host, "account_id": me.ID, "handle": handle}
	if len(limits) > 0 {
		meta["limits"] = limits
	}
	display := me.DisplayName
	if display == "" {
		display = me.Username
	}
	avatar := ""
	if strings.HasPrefix(me.Avatar, "https://") {
		avatar = me.Avatar
	}
	return provider.Profile{ID: host + ":" + me.ID, Username: handle, DisplayName: display, AvatarURL: avatar, Metadata: meta}, token, nil
}

// limits reads the instance limits: /api/v2/instance, then /api/v1/instance for
// servers without v2. A server that has neither simply stores no limits, so the
// defaults of Capabilities apply. Any other failure aborts the connect.
func (a *Adapter) limits(ctx context.Context, base, token string) (map[string]any, error) {
	for _, path := range []string{"/api/v2/instance", "/api/v1/instance"} {
		r, err := a.get(ctx, base, path, token)
		if err != nil {
			return nil, err
		}
		if r.status == http.StatusNotFound {
			continue
		}
		if r.status != http.StatusOK {
			return nil, a.failure(r, token)
		}
		var info instanceInfo
		if err := json.Unmarshal(r.body, &info); err != nil {
			return nil, &provider.Error{Kind: provider.KindPermanent, Provider: Name, Code: "NOT_MASTODON", Message: "Mastodon: the instance answered with unreadable data"}
		}
		out := map[string]any{}
		if n := info.Configuration.Statuses.MaxCharacters; n > 0 {
			out["max_characters"] = n
		}
		if n := info.Configuration.Statuses.MaxMedia; n > 0 {
			out["max_media"] = n
		}
		if n := info.Configuration.Media.ImageSizeLimit; n > 0 {
			out["max_image_bytes"] = n
		}
		return out, nil
	}
	return nil, nil
}
