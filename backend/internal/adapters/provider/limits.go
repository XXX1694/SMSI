package provider

import "math"

// WithAccountLimits narrows c by the per-account limits an adapter stored in
// the account metadata under "limits" (for example a Mastodon instance's
// max_characters). The stricter of the two always wins, so a stored value can
// never raise what the provider declares. Unknown or non-positive values are ignored.
//
// Recognised keys: max_characters, max_media, max_image_bytes.
func (c Capabilities) WithAccountLimits(metadata map[string]any) Capabilities {
	limits, ok := metadata["limits"].(map[string]any)
	if !ok {
		return c
	}
	if n, ok := positive(limits["max_characters"]); ok && (c.MaxTextLength == 0 || n < int64(c.MaxTextLength)) {
		c.MaxTextLength = int(n)
	}
	if n, ok := positive(limits["max_media"]); ok && n < int64(c.MaxMediaCount) {
		c.MaxMediaCount = int(n)
	}
	if n, ok := positive(limits["max_image_bytes"]); ok && (c.MaxImageBytes == 0 || n < c.MaxImageBytes) {
		c.MaxImageBytes = n
	}
	return c
}

// positive reads a JSON-decoded number (float64 from JSON, int/int64 from code) greater than zero.
func positive(v any) (int64, bool) {
	var n int64
	switch x := v.(type) {
	case int:
		n = int64(x)
	case int64:
		n = x
	case float64:
		if x != math.Trunc(x) || x > math.MaxInt32*1024 {
			return 0, false
		}
		n = int64(x)
	default:
		return 0, false
	}
	return n, n > 0
}
