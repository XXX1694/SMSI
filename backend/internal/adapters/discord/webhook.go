package discord

import (
	"net/url"
	"regexp"
	"strings"

	"github.com/socialos/backend/internal/adapters/provider"
)

var (
	idPattern    = regexp.MustCompile(`^[0-9]{5,25}$`)
	tokenPattern = regexp.MustCompile(`^[A-Za-z0-9._-]{1,200}$`)
	versionSeg   = regexp.MustCompile(`^v[0-9]{1,2}$`)
)

// webhook is a parsed webhook URL. token is the secret half.
type webhook struct{ host, id, token string }

// canonical is the stored credential: one normalised spelling per webhook.
func (w webhook) canonical() string {
	return "https://" + w.host + "/api/webhooks/" + w.id + "/" + w.token
}

func invalidWebhook() *provider.Error {
	return &provider.Error{Kind: provider.KindPermanent, Provider: Name, Code: "INVALID_WEBHOOK_URL",
		Message: "this is not a Discord webhook URL (expected https://discord.com/api/webhooks/<id>/<token>)"}
}

// parseWebhookURL accepts only https URLs on Discord's own hosts. Query and
// fragment are dropped; the error never repeats the input.
func parseWebhookURL(raw string) (webhook, error) {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || u.Scheme != "https" || u.User != nil || u.Port() != "" {
		return webhook{}, invalidWebhook()
	}
	host := strings.ToLower(u.Hostname())
	if host != "discord.com" && host != "discordapp.com" {
		return webhook{}, invalidWebhook()
	}
	segs := strings.Split(strings.Trim(u.Path, "/"), "/")
	if len(segs) == 5 && segs[0] == "api" && versionSeg.MatchString(segs[1]) {
		segs = append(segs[:1], segs[2:]...)
	}
	if len(segs) != 4 || segs[0] != "api" || segs[1] != "webhooks" || !idPattern.MatchString(segs[2]) || !tokenPattern.MatchString(segs[3]) {
		return webhook{}, invalidWebhook()
	}
	return webhook{host: host, id: segs[2], token: segs[3]}, nil
}

// url builds a webhook endpoint; the test base replaces the host.
func (a *Adapter) url(w webhook, suffix string) string {
	base := a.base
	if base == "" {
		base = "https://" + w.host
	}
	return base + "/api/webhooks/" + w.id + "/" + w.token + suffix
}
