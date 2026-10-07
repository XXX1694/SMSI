package http

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/socialos/backend/internal/application/port"
	"github.com/socialos/backend/internal/domain/errs"
	"github.com/socialos/backend/internal/domain/post"
)

func jsonReq(ct, body string) *http.Request {
	r := httptest.NewRequest("POST", "/", strings.NewReader(body))
	if ct != "" {
		r.Header.Set("Content-Type", ct)
	}
	return r
}

func TestDecode(t *testing.T) {
	type in struct {
		Name  string   `json:"name"`
		Count int      `json:"count"`
		IDs   []string `json:"ids"`
	}
	var v in
	if err := decode(jsonReq("application/json", `{"name":"x","count":3,"ids":["a"]}`), &v); err != nil || v.Name != "x" || v.Count != 3 || len(v.IDs) != 1 {
		t.Fatalf("%+v %v", v, err)
	}
	if err := decode(jsonReq("application/json; charset=utf-8", `{"name":"y"}`), &v); err != nil || v.Name != "y" {
		t.Fatalf("charset parameter: %v", err)
	}
	// Unknown fields are ignored (forward compatible clients).
	if err := decode(jsonReq("application/json", `{"name":"z","future":true}`), &v); err != nil || v.Name != "z" {
		t.Fatalf("unknown field: %v", err)
	}
	// Empty body decodes to the zero value, with or without a content type.
	v = in{}
	for _, ct := range []string{"", "application/json"} {
		if err := decode(jsonReq(ct, ""), &v); err != nil || v.Name != "" {
			t.Fatalf("empty body (%q): %v", ct, err)
		}
	}
	for name, tc := range map[string]struct{ ct, body string }{
		"malformed":     {"application/json", `{"name":`},
		"not an object": {"application/json", `[1,2]`},
		"wrong type":    {"application/json", `{"count":"three"}`},
		"form encoded":  {"application/x-www-form-urlencoded", `name=x`},
		"text":          {"text/plain", `{"name":"x"}`},
		"garbage":       {"application/json", `\x00\x01`},
	} {
		err := decode(jsonReq(tc.ct, tc.body), &in{})
		if e, ok := errs.As(err); !ok || e.Code != errs.Validation {
			t.Errorf("%s: want a validation error, got %v", name, err)
		}
	}
	// The field name of a type error is reported.
	err := decode(jsonReq("application/json", `{"count":"three"}`), &in{})
	if e, _ := errs.As(err); e == nil || e.Fields["count"] == "" {
		t.Errorf("type errors should name the field: %v", err)
	}
	// Bodies beyond 1 MiB are cut off rather than read to the end.
	big := `{"name":"` + strings.Repeat("a", maxJSONBody+10) + `"}`
	if err := decode(jsonReq("application/json", big), &in{}); err == nil {
		t.Error("oversized body accepted")
	}
	// Reading from a failing body is a validation error, not a panic.
	r := httptest.NewRequest("POST", "/", io.NopCloser(errReader{}))
	r.Header.Set("Content-Type", "application/json")
	r.ContentLength = 10
	if err := decode(r, &in{}); err == nil {
		t.Error("a failing reader must surface an error")
	}
}

type errReader struct{}

func (errReader) Read([]byte) (int, error) { return 0, errors.New("connection reset") }

func TestPathID(t *testing.T) {
	id := uuid.New()
	run := func(param string) (uuid.UUID, error) {
		var got uuid.UUID
		var err error
		rt := chi.NewRouter()
		rt.Get("/x/{id}", func(_ http.ResponseWriter, r *http.Request) { got, err = pathID(r, "id") })
		rt.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("GET", "/x/"+param, nil))
		return got, err
	}
	if got, err := run(id.String()); err != nil || got != id {
		t.Fatalf("%v %v", got, err)
	}
	// Malformed ids are NOT_FOUND (not 400), so id shapes cannot be probed.
	for _, bad := range []string{"nope", "123", "%27%20OR%201=1", id.String() + "x", "00000000-0000-0000-0000-00000000000g"} {
		if _, err := run(bad); !errs.Is(err, errs.NotFound) {
			t.Errorf("%q: %v", bad, err)
		}
	}
}

func TestPageParams(t *testing.T) {
	get := func(q string) (port.Page, error) { return pageParams(httptest.NewRequest("GET", "/?"+q, nil)) }
	if p, err := get(""); err != nil || p.Limit <= 0 || p.Cursor != nil {
		t.Fatalf("defaults: %+v %v", p, err)
	}
	def, _ := get("")
	if p, err := get("limit=5"); err != nil || p.Limit != 5 {
		t.Fatalf("explicit limit: %+v %v", p, err)
	}
	if p, err := get("limit=1000000"); err != nil || p.Limit > 100 || p.Limit < def.Limit {
		t.Fatalf("limit must be capped: %+v %v", p, err)
	}
	cur := port.Cursor{At: time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC), ID: uuid.New()}
	if p, err := get("cursor=" + cur.Encode()); err != nil || p.Cursor == nil || p.Cursor.ID != cur.ID || !p.Cursor.At.Equal(cur.At) {
		t.Fatalf("cursor: %+v %v", p, err)
	}
	for _, q := range []string{"limit=0", "limit=-1", "limit=abc", "limit=1.5", "limit=", "cursor=not-a-cursor", "cursor=!!!"} {
		_, err := get(q)
		if q == "limit=" {
			if err != nil {
				t.Errorf("an empty limit means default: %v", err)
			}
			continue
		}
		if e, ok := errs.As(err); !ok || e.Code != errs.Validation {
			t.Errorf("?%s: %v", q, err)
		}
	}
}

func TestQueryTime(t *testing.T) {
	get := func(q string) (*time.Time, error) { return queryTime(httptest.NewRequest("GET", "/?"+q, nil), "from") }
	if ts, err := get(""); ts != nil || err != nil {
		t.Fatalf("absent: %v %v", ts, err)
	}
	ts, err := get("from=2026-10-07T12:00:00%2B05:00")
	if err != nil || ts == nil || ts.Location() != time.UTC || !ts.Equal(time.Date(2026, 10, 7, 7, 0, 0, 0, time.UTC)) {
		t.Fatalf("offsets are converted to UTC: %v %v", ts, err)
	}
	if ts, err := get("from=2026-10-07T12:00:00.123456Z"); err != nil || ts.Nanosecond() != 123456000 {
		t.Fatalf("fractional seconds: %v %v", ts, err)
	}
	for _, bad := range []string{"from=yesterday", "from=2026-10-07", "from=2026-13-01T00:00:00Z", "from=1759838400", "from=2026-10-07%2012:00:00"} {
		if _, err := get(bad); err == nil {
			t.Errorf("%s accepted", bad)
		} else if e, _ := errs.As(err); e == nil || e.Fields["from"] == "" {
			t.Errorf("%s: %v", bad, err)
		}
	}
}

func TestUUIDs(t *testing.T) {
	a, b := uuid.New(), uuid.New()
	got, err := uuids("ids", []string{a.String(), b.String()})
	if err != nil || len(got) != 2 || got[0] != a || got[1] != b {
		t.Fatalf("%v %v", got, err)
	}
	if got, err := uuids("ids", nil); err != nil || got == nil || len(got) != 0 {
		t.Fatalf("nil input must give an empty, non-nil slice: %v %v", got, err)
	}
	_, err = uuids("media_ids", []string{a.String(), "bogus"})
	if e, _ := errs.As(err); e == nil || e.Code != errs.Validation || e.Fields["media_ids"] == "" {
		t.Fatalf("%v", err)
	}
}

func TestNewPageNeverHasNullItems(t *testing.T) {
	b, _ := json.Marshal(newPage[int](nil, ""))
	if string(b) != `{"items":[],"next_cursor":null}` {
		t.Fatalf("%s", b)
	}
	b, _ = json.Marshal(newPage([]int{1}, "abc"))
	if string(b) != `{"items":[1],"next_cursor":"abc"}` {
		t.Fatalf("%s", b)
	}
}

func TestDTOTimesAreUTC(t *testing.T) {
	zone := time.FixedZone("UTC+5", 5*3600)
	local := time.Date(2026, 10, 7, 12, 0, 0, 0, zone)
	if got := utc(local); got.Location() != time.UTC || !got.Equal(local) {
		t.Fatalf("%v", got)
	}
	if utcp(nil) != nil {
		t.Fatal("nil stays nil")
	}
	if got := utcp(&local); got.Location() != time.UTC || !got.Equal(local) {
		t.Fatalf("%v", got)
	}
	p := &post.Post{ID: uuid.New(), CreatedAt: local, UpdatedAt: local, ScheduledAt: &local, Status: post.StatusScheduled}
	b, _ := json.Marshal(toPost(p))
	var out map[string]any
	_ = json.Unmarshal(b, &out)
	for _, k := range []string{"created_at", "updated_at", "scheduled_at"} {
		if s, _ := out[k].(string); !strings.HasSuffix(s, "Z") || s != "2026-10-07T07:00:00Z" {
			t.Errorf("%s = %v", k, out[k])
		}
	}
	// Lists are [] and not null so clients can iterate without guards.
	if out["targets"] == nil || out["media_ids"] == nil {
		t.Errorf("empty lists must serialise as []: %s", b)
	}
	if _, has := out["media"]; has {
		t.Errorf("list items carry no media objects: %s", b)
	}
	d, _ := json.Marshal(toPostDetail(p, nil, nil))
	var detail map[string]any
	_ = json.Unmarshal(d, &detail)
	if a, ok := detail["attempts"].([]any); !ok || len(a) != 0 {
		t.Errorf("detail attempts must be []: %s", d)
	}
	if m, ok := detail["media"].([]any); !ok || len(m) != 0 {
		t.Errorf("detail media must be []: %s", d)
	}
}
