package media

import "testing"

func TestClassify(t *testing.T) {
	for mime, want := range map[string]struct {
		kind Kind
		ext  string
	}{
		"image/jpeg": {KindImage, ".jpg"}, "image/png": {KindImage, ".png"}, "image/webp": {KindImage, ".webp"}, "image/gif": {KindImage, ".gif"},
		"video/mp4": {KindVideo, ".mp4"}, "video/quicktime": {KindVideo, ".mov"},
	} {
		k, ext, ok := Classify(mime)
		if !ok || k != want.kind || ext != want.ext {
			t.Errorf("%s: %v %v %v", mime, k, ext, ok)
		}
	}
	for _, mime := range []string{"", "image/svg+xml", "image/bmp", "image/tiff", "image/heic", "video/webm", "application/pdf", "text/html",
		"application/octet-stream", "IMAGE/PNG", "image/png; charset=binary", "image/*"} {
		if _, _, ok := Classify(mime); ok {
			t.Errorf("%q must not be allowed", mime)
		}
	}
}

func TestMaxBytes(t *testing.T) {
	if MaxBytes(KindImage) != 10<<20 || MaxBytes(KindVideo) != 100<<20 {
		t.Fatalf("limits: %d %d", MaxBytes(KindImage), MaxBytes(KindVideo))
	}
	if MaxBytes(Kind("unknown")) != MaxImageBytes {
		t.Fatal("unknown kinds get the strictest limit")
	}
}
