package accounts

import (
	"net/url"
	"sort"

	"github.com/socialos/backend/internal/adapters/provider"
	"github.com/socialos/backend/internal/domain/approval"
	"github.com/socialos/backend/internal/domain/socialaccount"
)

func disconnectRequest(acc *socialaccount.Account) approval.Request {
	return approval.Request{Action: approval.ActionAccountDisconnect, ResourceType: "social_account", ResourceID: acc.ID.String(),
		Fingerprint: approval.Fingerprint(acc.ID.String()),
		Summary:     map[string]any{"provider": acc.Provider, "username": acc.Username}}
}

// connectRequest binds the approval to the provider and every submitted value, so the key cannot swap the credential
// after the owner agreed. The fingerprint is an HMAC over every value, secrets included; the summary never shows a secret or a full URL.
func connectRequest(key []byte, p provider.Provider, fields map[string]string) approval.Request {
	names := make([]string, 0, len(fields))
	for n := range fields {
		names = append(names, n)
	}
	sort.Strings(names)
	parts := []string{p.Name()}
	for _, n := range names {
		parts = append(parts, n, fields[n])
	}
	shown := map[string]any{"provider": p.DisplayName()}
	for _, f := range p.Capabilities().ConnectFields {
		switch v := fields[f.Name]; {
		case v == "":
		case f.Kind == provider.FieldText:
			shown[f.Name] = v
		case f.Kind == provider.FieldURL: // a URL may carry a secret (webhooks): show the host only
			if u, err := url.Parse(v); err == nil {
				shown[f.Name] = u.Hostname()
			}
		}
	}
	return approval.Request{Action: approval.ActionAccountConnect, ResourceType: "social_provider", ResourceID: p.Name(),
		Fingerprint: approval.KeyedFingerprint(key, parts...), Summary: shown}
}
