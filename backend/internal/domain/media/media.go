// Package media defines uploaded media and its validation rules.
package media

import (
	"time"

	"github.com/google/uuid"
)

// Kind of media.
type Kind string

const (
	KindImage Kind = "image"
	KindVideo Kind = "video"
)

// Size limits (ARCHITECTURE.md §4 Media).
const (
	MaxImageBytes int64 = 10 << 20
	MaxVideoBytes int64 = 100 << 20
)

// allowed is the MIME allow-list mapped to kind and canonical extension.
var allowed = map[string]struct {
	kind Kind
	ext  string
}{
	"image/jpeg":      {KindImage, ".jpg"},
	"image/png":       {KindImage, ".png"},
	"image/webp":      {KindImage, ".webp"},
	"image/gif":       {KindImage, ".gif"},
	"video/mp4":       {KindVideo, ".mp4"},
	"video/quicktime": {KindVideo, ".mov"},
}

// Classify returns kind and extension for an allowed MIME type.
func Classify(mime string) (Kind, string, bool) {
	a, ok := allowed[mime]
	return a.kind, a.ext, ok
}

// MaxBytes returns the size limit for a kind.
func MaxBytes(k Kind) int64 {
	if k == KindVideo {
		return MaxVideoBytes
	}
	return MaxImageBytes
}

// Status of a media object.
type Status string

const (
	StatusReady Status = "ready"
)

// Media is an uploaded file.
type Media struct {
	ID           uuid.UUID
	UserID       uuid.UUID
	Kind         Kind
	MimeType     string
	SizeBytes    int64
	StorageKey   string
	OriginalName string
	Width        int
	Height       int
	Status       Status
	SHA256       string
	CreatedAt    time.Time
}
