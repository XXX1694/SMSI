// Package stubs registers networks that are NOT implemented in the MVP.
// They are listed so clients can show them as "coming soon / needs approval",
// and every operation fails with PROVIDER_NOT_AVAILABLE. They never pretend to work.
package stubs

import "github.com/socialos/backend/internal/adapters/provider"

// Unsupported is a clearly-labelled placeholder provider.
type Unsupported struct {
	name, display, notes string
}

func (u Unsupported) Name() string        { return u.name }
func (u Unsupported) DisplayName() string { return u.display }
func (u Unsupported) Configured() bool    { return false }
func (u Unsupported) Supported() bool     { return false }

// Capabilities reports nothing publishable and RequiresApproval=true.
func (u Unsupported) Capabilities() provider.Capabilities {
	return provider.Capabilities{
		RequiresApproval: true,
		ConnectMethod:    provider.ConnectNone,
		Notes:            "Not available yet: " + u.notes,
	}
}

// All returns the stub providers.
func All() []provider.Provider {
	return []provider.Provider{
		Unsupported{"instagram", "Instagram", "needs an Instagram Business or Creator account. Posting for other people needs Meta review."},
		Unsupported{"facebook", "Facebook", "posting to Pages needs Meta app review."},
		Unsupported{"tiktok", "TikTok", "until TikTok audits the app, posts can only be private."},
		Unsupported{"youtube", "YouTube", "until Google verifies the app, uploads can only be private."},
		Unsupported{"x", "X (Twitter)", "X charges per post through its paid API."},
		Unsupported{"threads", "Threads", "needs Meta app review."},
		Unsupported{"pinterest", "Pinterest", "needs Pinterest API approval."},
		Unsupported{"reddit", "Reddit", "new API access reportedly needs Reddit approval first."},
		Unsupported{"medium", "Medium", "Medium reportedly no longer issues new integration tokens."},
		Unsupported{"hashnode", "Hashnode", "Hashnode reportedly needs a paid Pro plan for API access."},
	}
}
