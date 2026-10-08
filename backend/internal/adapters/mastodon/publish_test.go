package mastodon

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/socialos/backend/internal/adapters/provider"
)

func okStatus(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, 200, `{"id":"109","url":"https://example.com/@alice/109","uri":"https://example.com/users/alice/statuses/109"}`)
}

func TestPublishText(t *testing.T) {
	f := newFake(t)
	f.on("POST /api/v1/statuses", okStatus)
	res, err := f.adapter().Publish(context.Background(), publishReq("hello fediverse"))
	if err != nil {
		t.Fatal(err)
	}
	if res.ExternalID != "109" || res.URL != "https://example.com/@alice/109" {
		t.Fatalf("%+v", res)
	}
	got := f.requests("POST /api/v1/statuses")[0]
	if got.Header.Get("Authorization") != "Bearer "+testToken || got.Header.Get("Idempotency-Key") != "socialos-target-uuid-1" {
		t.Fatalf("headers: %v", got.Header)
	}
	var body map[string]any
	_ = json.Unmarshal([]byte(got.Body), &body)
	if body["status"] != "hello fediverse" || body["visibility"] != "public" || body["media_ids"] != nil {
		t.Fatalf("body: %s", got.Body)
	}
}

func TestPublishSendsTheSameIdempotencyKeyOnEveryAttempt(t *testing.T) {
	f := newFake(t)
	f.on("POST /api/v1/statuses", okStatus)
	a := f.adapter()
	for i := 0; i < 2; i++ {
		if _, err := a.Publish(context.Background(), publishReq("same")); err != nil {
			t.Fatal(err)
		}
	}
	reqs := f.requests("POST /api/v1/statuses")
	if len(reqs) != 2 || reqs[0].Header.Get("Idempotency-Key") != reqs[1].Header.Get("Idempotency-Key") {
		t.Fatalf("keys differ: %v", reqs)
	}
	other := publishReq("same")
	other.IdempotencyKey = "target-uuid-2"
	_, _ = a.Publish(context.Background(), other)
	if f.requests("POST /api/v1/statuses")[2].Header.Get("Idempotency-Key") == reqs[0].Header.Get("Idempotency-Key") {
		t.Fatal("different targets must get different keys")
	}
	req := publishReq("x")
	req.IdempotencyKey = " "
	if _, err := a.Publish(context.Background(), req); provider.Classify(err) != provider.KindPermanent {
		t.Fatalf("a missing key must be refused, got %v", err)
	}
}

func TestPublishWithImageProcessedSynchronously(t *testing.T) {
	f := newFake(t)
	f.on("POST /api/v2/media", func(w http.ResponseWriter, _ *http.Request) { writeJSON(w, 200, `{"id":"m1","type":"image"}`) })
	f.on("POST /api/v1/statuses", okStatus)
	if _, err := f.adapter().Publish(context.Background(), publishReq("with pic", imageFile("PNGDATA"))); err != nil {
		t.Fatal(err)
	}
	up := f.requests("POST /api/v2/media")[0]
	if !strings.HasPrefix(up.Header.Get("Content-Type"), "multipart/form-data") || !strings.Contains(up.Body, "PNGDATA") || !strings.Contains(up.Body, `name="file"`) {
		t.Fatalf("upload: %v %q", up.Header, up.Body)
	}
	if !strings.Contains(f.requests("POST /api/v1/statuses")[0].Body, `"media_ids":["m1"]`) {
		t.Fatalf("status body: %s", f.requests("POST /api/v1/statuses")[0].Body)
	}
}

func TestPublishWaitsForAsyncMedia(t *testing.T) {
	f := newFake(t)
	var polls int32
	f.on("POST /api/v2/media", func(w http.ResponseWriter, _ *http.Request) { writeJSON(w, 202, `{"id":"m2","url":null}`) })
	f.on("GET /api/v1/media/m2", func(w http.ResponseWriter, _ *http.Request) {
		if atomic.AddInt32(&polls, 1) < 3 {
			writeJSON(w, 206, `{"id":"m2","url":null}`)
			return
		}
		writeJSON(w, 200, `{"id":"m2","url":"https://example.com/m2.png"}`)
	})
	f.on("POST /api/v1/statuses", okStatus)
	if _, err := f.adapter().Publish(context.Background(), publishReq("async", imageFile("x"))); err != nil {
		t.Fatal(err)
	}
	if polls != 3 {
		t.Fatalf("polls = %d, want 3", polls)
	}
	if len(f.requests("POST /api/v1/statuses")) != 1 {
		t.Fatal("the status must be created once, after the media is ready")
	}
}

func TestAsyncMediaStopsOnTimeoutAndContext(t *testing.T) {
	f := newFake(t)
	f.on("POST /api/v2/media", func(w http.ResponseWriter, _ *http.Request) { writeJSON(w, 202, `{"id":"m3"}`) })
	f.on("GET /api/v1/media/m3", func(w http.ResponseWriter, _ *http.Request) { writeJSON(w, 206, `{"id":"m3"}`) })
	f.on("POST /api/v1/statuses", okStatus)
	a := f.adapter()
	a.cfg.MediaProcessingTimeout = 30 * time.Millisecond
	_, err := a.Publish(context.Background(), publishReq("slow", imageFile("x")))
	if provider.Classify(err) != provider.KindRetryable || provider.CodeOf(err) != "MEDIA_PROCESSING_TIMEOUT" {
		t.Fatalf("got %v", err)
	}
	a.cfg.MediaProcessingTimeout = time.Minute
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()
	if _, err := a.Publish(ctx, publishReq("slow", imageFile("x"))); provider.Classify(err) != provider.KindUnknown {
		t.Fatalf("a cancelled wait is Unknown, got %v", err)
	}
	if len(f.requests("POST /api/v1/statuses")) != 0 {
		t.Fatal("no status may be created while media is unfinished")
	}
}

func TestPublishFallsBackToMediaV1(t *testing.T) {
	f := newFake(t)
	f.on("POST /api/v1/media", func(w http.ResponseWriter, _ *http.Request) { writeJSON(w, 200, `{"id":"m4"}`) })
	f.on("POST /api/v1/statuses", okStatus)
	if _, err := f.adapter().Publish(context.Background(), publishReq("old server", imageFile("x"))); err != nil {
		t.Fatal(err)
	}
	if len(f.requests("POST /api/v1/media")) != 1 {
		t.Fatal("v1 upload expected")
	}
}

func TestPublishRefusesVideo(t *testing.T) {
	f := newFake(t)
	m := imageFile("x")
	m.Kind = "video"
	if _, err := f.adapter().Publish(context.Background(), publishReq("v", m)); provider.Classify(err) != provider.KindPermanent {
		t.Fatalf("got %v", err)
	}
}

func TestDelete(t *testing.T) {
	f := newFake(t)
	f.on("DELETE /api/v1/statuses/109", func(w http.ResponseWriter, _ *http.Request) { writeJSON(w, 200, `{"id":"109"}`) })
	a := f.adapter()
	req := provider.DeleteRequest{Account: account(), AccessToken: testToken, ExternalID: "109"}
	if err := a.Delete(context.Background(), req); err != nil {
		t.Fatal(err)
	}
	if f.requests("DELETE /api/v1/statuses/109")[0].Header.Get("Authorization") != "Bearer "+testToken {
		t.Fatal("token header missing")
	}
	req.ExternalID = "gone" // 404 from the fake: already deleted
	if err := a.Delete(context.Background(), req); err != nil {
		t.Fatalf("an already deleted status is not an error: %v", err)
	}
	f.on("DELETE /api/v1/statuses/403", func(w http.ResponseWriter, _ *http.Request) { writeJSON(w, 403, `{"error":"not yours"}`) })
	req.ExternalID = "403"
	if err := a.Delete(context.Background(), req); provider.Classify(err) != provider.KindAuth {
		t.Fatalf("got %v", err)
	}
	req.ExternalID = ""
	if err := a.Delete(context.Background(), req); provider.Classify(err) != provider.KindPermanent {
		t.Fatalf("got %v", err)
	}
}

func TestAccountWithoutHostNeedsReconnect(t *testing.T) {
	f := newFake(t)
	req := publishReq("x")
	req.Account = provider.AccountRef{ID: "a"}
	if _, err := f.adapter().Publish(context.Background(), req); provider.Classify(err) != provider.KindAuth {
		t.Fatalf("got %v", err)
	}
}
