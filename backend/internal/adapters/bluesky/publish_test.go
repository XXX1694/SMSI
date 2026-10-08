package bluesky

import (
	"bytes"
	"context"
	"io"
	"regexp"
	"strings"
	"testing"

	"github.com/socialos/backend/internal/adapters/provider"
)

func TestPublishBuildsRecordWithFacetsAndURL(t *testing.T) {
	f := newFake(t)
	text := "Привет 👨‍👩‍👧 see https://example.com/a?b=1. #go"
	res, err := f.adapter().Publish(context.Background(), req(text))
	if err != nil {
		t.Fatal(err)
	}
	rec := f.records[res.ExternalID]
	if rec["$type"] != postType || rec["text"] != text || rec["createdAt"] == "" {
		t.Fatalf("record %v", rec)
	}
	if res.URL != "https://bsky.app/profile/"+testHandle+"/post/"+res.ExternalID {
		t.Errorf("url %q", res.URL)
	}
	facets := rec["facets"].([]any)
	if len(facets) != 2 {
		t.Fatalf("facets %v", facets)
	}
	idx := facets[0].(map[string]any)["index"].(map[string]any)
	s, e := int(idx["byteStart"].(float64)), int(idx["byteEnd"].(float64))
	if got := text[s:e]; got != "https://example.com/a?b=1" {
		t.Errorf("link slice %q", got)
	}
}

func TestFacetByteOffsetsAreUTF8(t *testing.T) {
	text := "héllo 🚀 world https://x.io/é (see #тег!) end"
	fs := buildFacets(text)
	if len(fs) != 2 {
		t.Fatalf("facets %+v", fs)
	}
	link := text[fs[0].Index.ByteStart:fs[0].Index.ByteEnd]
	tag := text[fs[1].Index.ByteStart:fs[1].Index.ByteEnd]
	if link != "https://x.io/é" || fs[0].Features[0].URI != link {
		t.Errorf("link %q", link)
	}
	if tag != "#тег" || fs[1].Features[0].Tag != "тег" {
		t.Errorf("tag %q %+v", tag, fs[1])
	}
	// Rune offsets would differ: the emoji alone is 4 bytes but 1 rune.
	if fs[0].Index.ByteStart == len([]rune(text[:fs[0].Index.ByteStart])) {
		t.Error("offsets must be bytes, not runes")
	}
}

func TestFacetEdgeCases(t *testing.T) {
	cases := map[string][]string{
		"(https://a.io/x_(y))":    {"https://a.io/x_(y)"},
		"(https://a.io/x)":        {"https://a.io/x"},
		"no link here":            nil,
		"http:// alone":           nil,
		"cost #123 and C#":        nil,
		"a#b not a tag":           nil,
		"https://a.io/#anchor ok": {"https://a.io/#anchor"},
	}
	for text, want := range cases {
		var got []string
		for _, f := range buildFacets(text) {
			got = append(got, text[f.Index.ByteStart:f.Index.ByteEnd])
		}
		if strings.Join(got, "|") != strings.Join(want, "|") {
			t.Errorf("%q: got %q want %q", text, got, want)
		}
	}
}

func TestGraphemeLimitIsCountedInGraphemes(t *testing.T) {
	family := "👨‍👩‍👧" // 1 grapheme, 5 runes
	if graphemeCount(family) != 1 || len([]rune(family)) != 5 {
		t.Fatal("test premise broken")
	}
	// 300 graphemes of 18 bytes would be 5400 bytes: the byte cap applies first.
	ok := strings.Repeat("a", 299) + family
	if err := checkContent(req(ok)); err != nil {
		t.Errorf("300 graphemes must pass: %v", err)
	}
	if err := checkContent(req(ok + "b")); provider.Classify(err) != provider.KindPermanent {
		t.Errorf("301 graphemes must fail: %v", err)
	}
	if err := checkContent(req(strings.Repeat(family, 300))); err == nil {
		t.Error("300 family emoji exceed 3000 bytes and must fail")
	}
	if err := checkContent(req("")); err == nil {
		t.Error("an empty post must fail")
	}
}

func TestRkeyIsDeterministicTID(t *testing.T) {
	a, b, c := rkeyFor("k1", t0), rkeyFor("k1", t0.Add(1e9)), rkeyFor("k2", t0)
	if a != b || a == c {
		t.Errorf("a=%s b=%s c=%s", a, b, c)
	}
	for _, k := range []string{a, c, rkeyFor("", t0)} {
		if !regexp.MustCompile(`^[234567abcdefghij][234567abcdefghijklmnopqrstuvwxyz]{12}$`).MatchString(k) {
			t.Errorf("not a TID: %q", k)
		}
	}
}

func TestPublishIsIdempotentAndLookupResolves(t *testing.T) {
	f := newFake(t)
	a := f.adapter()
	ctx := context.Background()
	if _, found, err := a.Lookup(ctx, req("x")); err != nil || found {
		t.Fatalf("before publish: found=%v err=%v", found, err)
	}
	first, err := a.Publish(ctx, req("x"))
	if err != nil {
		t.Fatal(err)
	}
	second, err := a.Publish(ctx, req("x")) // the PDS refuses the duplicate rkey; the adapter confirms it exists
	if err != nil || second.ExternalID != first.ExternalID || len(f.records) != 1 {
		t.Fatalf("second publish: %+v %v records=%d", second, err, len(f.records))
	}
	got, found, err := a.Lookup(ctx, req("x"))
	if err != nil || !found || got.ExternalID != first.ExternalID || got.URL != first.URL {
		t.Fatalf("lookup %+v found=%v err=%v", got, found, err)
	}
	if _, found, _ := a.Lookup(ctx, provider.PublishRequest{Account: account(), AccessToken: testPassword}); found {
		t.Error("a request without a key can never be found")
	}
}

func TestDeleteRemovesRecord(t *testing.T) {
	f := newFake(t)
	a := f.adapter()
	res, err := a.Publish(context.Background(), req("bye"))
	if err != nil {
		t.Fatal(err)
	}
	if err := a.Delete(context.Background(), provider.DeleteRequest{Account: account(), AccessToken: testPassword, ExternalID: res.ExternalID}); err != nil {
		t.Fatal(err)
	}
	if len(f.records) != 0 || len(f.deletes) != 1 {
		t.Errorf("records %d deletes %v", len(f.records), f.deletes)
	}
}

func media(n int, size int) []provider.MediaFile {
	var out []provider.MediaFile
	for i := 0; i < n; i++ {
		out = append(out, provider.MediaFile{Kind: "image", MimeType: "image/png", Size: int64(size),
			Open: func(context.Context) (io.ReadCloser, error) {
				return io.NopCloser(bytes.NewReader(make([]byte, size))), nil
			}})
	}
	return out
}

func TestPublishUploadsImagesAndEmbedsThem(t *testing.T) {
	f := newFake(t)
	r := req("pics")
	r.Media = media(2, 100)
	res, err := f.adapter().Publish(context.Background(), r)
	if err != nil {
		t.Fatal(err)
	}
	embed := f.records[res.ExternalID]["embed"].(map[string]any)
	imgs := embed["images"].([]any)
	if embed["$type"] != "app.bsky.embed.images" || len(imgs) != 2 || f.blobs != 2 {
		t.Fatalf("embed %v blobs %d", embed, f.blobs)
	}
	first := imgs[0].(map[string]any)
	if _, ok := first["alt"]; !ok || first["image"].(map[string]any)["mimeType"] != "image/png" {
		t.Errorf("image %v", first)
	}
}

func TestPublishRejectsBadMediaBeforeAnyRequest(t *testing.T) {
	f := newFake(t)
	a := f.adapter()
	for name, m := range map[string][]provider.MediaFile{
		"five images": media(5, 10),
		"too large":   media(1, MaxImageBytes+1),
		"video":       {{Kind: "video", MimeType: "video/mp4", Size: 10}},
	} {
		r := req("x")
		r.Media = m
		if _, err := a.Publish(context.Background(), r); provider.Classify(err) != provider.KindPermanent {
			t.Errorf("%s: %v", name, err)
		}
	}
	if f.creates != 0 {
		t.Error("validation must run before logging in")
	}
}

func TestNoSecretOrJWTInErrorsOrResults(t *testing.T) {
	f := newFake(t)
	a := f.adapter()
	res, err := a.Publish(context.Background(), req("ok"))
	if err != nil {
		t.Fatal(err)
	}
	f.failRecord = 500
	r := req("again")
	r.IdempotencyKey = "other"
	_, err2 := a.Publish(context.Background(), r)
	blob := err2.Error() + provider.SafeMessage(err2) + res.URL + res.ExternalID
	for k, v := range res.Metadata {
		blob += k + "=" + v.(string)
	}
	for _, bad := range []string{testPassword, "Bearer", "h.", "accessJwt", "refreshJwt"} {
		if strings.Contains(blob, bad) {
			t.Errorf("leaked %q in %q", bad, blob)
		}
	}
}
