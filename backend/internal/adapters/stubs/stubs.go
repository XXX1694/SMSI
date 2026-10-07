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
		Notes:            "UNSUPPORTED in this release: " + u.notes,
	}
}

// All returns the stub providers.
func All() []provider.Provider {
	return []provider.Provider{
		Unsupported{"instagram", "Instagram", "requires Meta app review for instagram_content_publish and a Business/Creator account."},
		Unsupported{"facebook", "Facebook", "requires Meta app review for pages_manage_posts."},
		Unsupported{"tiktok", "TikTok", "requires TikTok Content Posting API audit; unaudited apps can only post privately."},
		Unsupported{"youtube", "YouTube", "requires Google OAuth verification for youtube.upload scope."},
		Unsupported{"x", "X (Twitter)", "requires a paid X API tier with write access."},
		Unsupported{"threads", "Threads", "requires Meta app review for threads_content_publish."},
		Unsupported{"pinterest", "Pinterest", "requires Pinterest API standard access approval."},
	}
}
