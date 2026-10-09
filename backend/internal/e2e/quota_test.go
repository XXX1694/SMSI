package e2e

import (
	"bytes"
	"fmt"
	"image"
	"image/png"
	"math/rand"
	"net/http"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/socialos/backend/internal/config"
)

func withQuota(q config.QuotaConfig) envOpts {
	return envOpts{mutate: func(c *config.Config) { c.QuotaConfig = q }}
}

var unlimited = config.QuotaConfig{QuotaAccounts: -1, QuotaPostsPerMonth: -1, QuotaMediaMB: -1}

func quotaOf(q config.QuotaConfig, f func(*config.QuotaConfig)) config.QuotaConfig { f(&q); return q }

// noisePNG is a valid PNG of roughly 270 KB that does not compress.
func noisePNG(t *testing.T, seed int64) []byte {
	t.Helper()
	rng := rand.New(rand.NewSource(seed))
	img := image.NewNRGBA(image.Rect(0, 0, 260, 260))
	for i := range img.Pix {
		img.Pix[i] = byte(rng.Intn(256))
	}
	var b bytes.Buffer
	if err := png.Encode(&b, img); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}

func tokenKey(i int) string { return fmt.Sprintf("mt_quota_%016d", i) } // gitleaks:allow (fake test value)

func usageOf(t *testing.T, c *client, metric string) (used, limit float64) {
	t.Helper()
	q := c.must("GET", "/api/v1/account/usage", nil, 200)["quotas"].(map[string]any)[metric].(map[string]any)
	used, _ = q["used"].(float64)
	return used, q["limit"].(float64)
}

func requireQuotaExceeded(t *testing.T, r resp, field string) {
	t.Helper()
	if r.status != http.StatusForbidden || r.errCode(t) != "QUOTA_EXCEEDED" {
		t.Fatalf("want 403 QUOTA_EXCEEDED, got %d %s", r.status, r.body)
	}
	if got := r.errBody(t)["fields"].(map[string]any)["quota"]; got != field {
		t.Fatalf("quota field %v, want %s", got, field)
	}
}

func TestQuotaConnectedAccounts(t *testing.T) {
	e := newEnv(t, withQuota(quotaOf(unlimited, func(q *config.QuotaConfig) { q.QuotaAccounts = 2 })))
	c := e.browser()
	c.register("accounts-quota@example.com")
	first := c.connectToken(tokenKey(1))
	c.connectToken(tokenKey(2))

	requireQuotaExceeded(t, c.do("POST", tokenPath, tokenBody(tokenKey(3))), "connected_accounts")
	if used, limit := usageOf(t, c, "connected_accounts"); used != 2 || limit != 2 {
		t.Fatalf("usage %v of %v", used, limit)
	}
	// Reconnecting an account you already have is free.
	c.must("POST", tokenPath, tokenBody(tokenKey(1)), 201)
	// Disconnecting frees the slot.
	c.must("DELETE", "/api/v1/social/accounts/"+first, nil, 204)
	c.connectToken(tokenKey(3))
}

func TestQuotaPostsPerMonth(t *testing.T) {
	e := newEnv(t, withQuota(quotaOf(unlimited, func(q *config.QuotaConfig) { q.QuotaPostsPerMonth = 2 })))
	c := e.browser()
	c.register("posts-quota@example.com")
	acc := c.connectMock()
	at := fmtTime(time.Now().Add(48 * time.Hour))
	draft := func(n int) string {
		return c.must("POST", "/api/v1/posts", map[string]any{"content": fmt.Sprintf("post %d", n), "social_account_ids": []string{acc}}, 201)["id"].(string)
	}
	p1 := draft(1)
	c.must("POST", "/api/v1/posts/"+p1+"/schedule", map[string]any{"scheduled_at": at}, 200)
	// Unscheduling and scheduling the same post again does not count it twice.
	c.must("POST", "/api/v1/posts/"+p1+"/unschedule", nil, 200)
	c.must("POST", "/api/v1/posts/"+p1+"/schedule", map[string]any{"scheduled_at": at}, 200)
	if used, _ := usageOf(t, c, "scheduled_posts_month"); used != 1 {
		t.Fatalf("used %v, want 1", used)
	}
	// Creating a post already scheduled counts too.
	c.must("POST", "/api/v1/posts", map[string]any{"content": "second", "social_account_ids": []string{acc}, "schedule": true, "scheduled_at": at}, 201)

	p3 := draft(3) // drafts are free
	requireQuotaExceeded(t, c.do("POST", "/api/v1/posts/"+p3+"/schedule", map[string]any{"scheduled_at": at}), "scheduled_posts_month")
	requireQuotaExceeded(t, c.do("POST", "/api/v1/posts/"+p3+"/publish", nil), "scheduled_posts_month")
	requireQuotaExceeded(t, c.do("POST", "/api/v1/posts", map[string]any{"content": "fourth", "social_account_ids": []string{acc}, "schedule": true, "scheduled_at": at}), "scheduled_posts_month")
	if st := c.must("GET", "/api/v1/posts/"+p3+"/status", nil, 200)["status"]; st != "draft" {
		t.Fatalf("a refused post must stay a draft, is %v", st)
	}
	// Deleting a counted post does not give the slot back.
	c.must("DELETE", "/api/v1/posts/"+p1, nil, 204)
	requireQuotaExceeded(t, c.do("POST", "/api/v1/posts/"+p3+"/schedule", map[string]any{"scheduled_at": at}), "scheduled_posts_month")
}

func TestQuotaMediaBytes(t *testing.T) {
	e := newEnv(t, withQuota(quotaOf(unlimited, func(q *config.QuotaConfig) { q.QuotaMediaMB = 1 })))
	c := e.browser()
	c.register("media-quota@example.com")
	var ids []string
	for i := 0; i < 3; i++ {
		r := c.upload("n.png", noisePNG(t, int64(i)))
		if r.status != 201 {
			break
		}
		ids = append(ids, r.json(t)["id"].(string))
	}
	if len(ids) != 3 {
		// 3 x ~260 KB fits in 1 MB; the fourth must not.
		t.Fatalf("uploaded %d files, want 3", len(ids))
	}
	requireQuotaExceeded(t, c.upload("n.png", noisePNG(t, 9)), "media_bytes")
	if used, limit := usageOf(t, c, "media_bytes"); used >= limit || used < 600<<10 {
		t.Fatalf("usage %v of %v", used, limit)
	}
	if e.storage.Len() != 3 {
		t.Fatalf("a refused upload must leave no object behind, have %d", e.storage.Len())
	}
	c.must("DELETE", "/api/v1/media/"+ids[0], nil, 204)
	if r := c.upload("n.png", noisePNG(t, 9)); r.status != 201 {
		t.Fatalf("deleting media must free space: %d %s", r.status, r.body)
	}
}

// Concurrent requests must not all pass the check before any of them records its change.
func TestQuotaHoldsUnderConcurrency(t *testing.T) {
	e := newEnv(t, withQuota(config.QuotaConfig{QuotaAccounts: 3, QuotaPostsPerMonth: 2, QuotaMediaMB: 1}))
	c := e.browser()
	c.register("race-quota@example.com")
	acc := c.connectToken(tokenKey(0))
	at := fmtTime(time.Now().Add(48 * time.Hour))

	race := func(n int, f func(i int) bool) int {
		var ok atomic.Int32
		var wg sync.WaitGroup
		for i := 0; i < n; i++ {
			wg.Add(1)
			go func(i int) {
				defer wg.Done()
				if f(i) {
					ok.Add(1)
				}
			}(i)
		}
		wg.Wait()
		return int(ok.Load())
	}
	if got := race(8, func(i int) bool { return c.do("POST", tokenPath, tokenBody(tokenKey(i+1))).status == 201 }); got != 2 {
		t.Errorf("%d concurrent connects passed, want exactly 2 (3 slots, 1 used)", got)
	}
	if got := race(8, func(i int) bool {
		return c.do("POST", "/api/v1/posts", map[string]any{"title": "T", "content": fmt.Sprint(i), "social_account_ids": []string{acc}, "schedule": true, "scheduled_at": at}).status == 201
	}); got != 2 {
		t.Errorf("%d concurrent scheduled posts passed, want exactly 2", got)
	}
	png := noisePNG(t, 1)
	// One upload per user runs at a time (D-015), so most of these are refused with 429 before they reach the quota;
	// what matters is that the limit (3 files) is never exceeded.
	if got := race(8, func(i int) bool { return c.upload("n.png", png).status == 201 }); got < 1 || got > 3 {
		t.Errorf("%d concurrent uploads passed, want 1 to 3", got)
	}
}

func TestQuotaUnlimitedAndUsageReport(t *testing.T) {
	e := newEnv(t, withQuota(unlimited))
	c := e.browser()
	c.register("unlimited@example.com")
	for i := 0; i < 7; i++ {
		c.connectToken(tokenKey(i))
	}
	rep := c.must("GET", "/api/v1/account/usage", nil, 200)
	if rep["plan"] != "free" || rep["period_start"] == nil || rep["period_end"] == nil {
		t.Fatalf("report: %v", rep)
	}
	q := rep["quotas"].(map[string]any)
	if acc := q["connected_accounts"].(map[string]any); acc["used"].(float64) != 7 || acc["limit"].(float64) != -1 {
		t.Fatalf("accounts: %v", acc)
	}
	// A key needs analytics:read to see the usage.
	denied := e.apiKeyClient(c.createKey("noscope", "posts:read"))
	if r := denied.do("GET", "/api/v1/account/usage", nil); r.status != 403 || r.errCode(t) != "INSUFFICIENT_SCOPE" {
		t.Fatalf("want 403 INSUFFICIENT_SCOPE, got %d %s", r.status, r.body)
	}
	e.apiKeyClient(c.createKey("reader", "analytics:read")).must("GET", "/api/v1/account/usage", nil, 200)
}

type steppingClock struct{ t atomic.Int64 }

func (c *steppingClock) Now() time.Time  { return time.Unix(0, c.t.Load()).UTC() }
func (c *steppingClock) set(t time.Time) { c.t.Store(t.UnixNano()) }

// A post counts in the month it is scheduled or published in: scheduling at the end of one month must not bank an
// allowance for the next, while unscheduling and scheduling again within a month stays free.
func TestQuotaMonthRollover(t *testing.T) {
	clk := &steppingClock{}
	clk.set(time.Date(2026, 10, 31, 12, 0, 0, 0, time.UTC))
	e := newEnv(t, envOpts{clock: clk, mutate: func(c *config.Config) {
		c.QuotaConfig = quotaOf(unlimited, func(q *config.QuotaConfig) { q.QuotaPostsPerMonth = 1 })
		c.SessionTTL = 90 * 24 * time.Hour
	}})
	c := e.browser()
	c.register("rollover@example.com")
	acc := c.connectMock()
	at := func(d time.Duration) string { return fmtTime(clk.Now().Add(d)) }
	draft := func(n string) string {
		return c.must("POST", "/api/v1/posts", map[string]any{"content": n, "social_account_ids": []string{acc}}, 201)["id"].(string)
	}
	p1, p2 := draft("one"), draft("two")
	c.must("POST", "/api/v1/posts/"+p1+"/schedule", map[string]any{"scheduled_at": at(72 * time.Hour)}, 200) // counted in October
	c.must("POST", "/api/v1/posts/"+p1+"/unschedule", nil, 200)
	c.must("POST", "/api/v1/posts/"+p1+"/schedule", map[string]any{"scheduled_at": at(72 * time.Hour)}, 200) // same month: free
	c.must("POST", "/api/v1/posts/"+p1+"/unschedule", nil, 200)
	requireQuotaExceeded(t, c.do("POST", "/api/v1/posts/"+p2+"/schedule", map[string]any{"scheduled_at": at(72 * time.Hour)}), "scheduled_posts_month")

	clk.set(time.Date(2026, 11, 2, 9, 0, 0, 0, time.UTC))
	c.must("POST", "/api/v1/posts/"+p2+"/schedule", map[string]any{"scheduled_at": at(72 * time.Hour)}, 200) // November's one slot
	// p1 was counted in October; scheduling it now uses a November slot, which is taken.
	requireQuotaExceeded(t, c.do("POST", "/api/v1/posts/"+p1+"/schedule", map[string]any{"scheduled_at": at(72 * time.Hour)}), "scheduled_posts_month")
	if used, _ := usageOf(t, c, "scheduled_posts_month"); used != 1 {
		t.Fatalf("November usage %v, want 1", used)
	}
}

// The owner is not asked to approve what the plan would refuse anyway.
func TestQuotaIsCheckedBeforeApproval(t *testing.T) {
	e := newEnv(t, withQuota(quotaOf(unlimited, func(q *config.QuotaConfig) { q.QuotaPostsPerMonth = 1 })))
	c := e.browser()
	c.register("approval-order@example.com")
	acc := c.connectMock()
	at := fmtTime(time.Now().Add(48 * time.Hour))
	c.must("POST", "/api/v1/posts", map[string]any{"content": "full", "social_account_ids": []string{acc}, "schedule": true, "scheduled_at": at}, 201)
	p2 := c.must("POST", "/api/v1/posts", map[string]any{"content": "next", "social_account_ids": []string{acc}}, 201)["id"].(string)
	k := e.apiKeyClient(c.createKey("agent", "posts:read", "posts:write", "posts:schedule", "posts:publish"))
	requireQuotaExceeded(t, k.do("POST", "/api/v1/posts/"+p2+"/publish", nil), "scheduled_posts_month")
	requireQuotaExceeded(t, k.do("POST", "/api/v1/posts/"+p2+"/schedule", map[string]any{"scheduled_at": fmtTime(time.Now().Add(time.Minute))}), "scheduled_posts_month")
}

// At the limit, starting to connect a network you do not have yet is refused up front, not after the consent screen.
func TestQuotaRefusesAnOAuthStartAtTheLimit(t *testing.T) {
	e := newEnv(t, withQuota(quotaOf(unlimited, func(q *config.QuotaConfig) { q.QuotaAccounts = 1 })))
	c := e.browser()
	c.register("oauth-quota@example.com")
	c.connectToken(tokenKey(1))
	requireQuotaExceeded(t, c.do("GET", "/api/v1/social/mock/connect?redirect=/accounts", nil), "connected_accounts")
}

// The size of a streamed upload is only known after it was read: it is checked in the transaction of the insert, and a
// refused file leaves no object behind. A file under the limit and an unlimited plan both pass.
func TestQuotaStreamedUploadOverTheLimit(t *testing.T) {
	big := func() []byte {
		rng := rand.New(rand.NewSource(7))
		img := image.NewNRGBA(image.Rect(0, 0, 560, 560))
		for i := range img.Pix {
			img.Pix[i] = byte(rng.Intn(256))
		}
		var b bytes.Buffer
		if err := png.Encode(&b, img); err != nil {
			t.Fatal(err)
		}
		return b.Bytes()
	}()
	if len(big) <= 1<<20 {
		t.Fatalf("test image is only %d bytes", len(big))
	}
	e := newEnv(t, withQuota(quotaOf(unlimited, func(q *config.QuotaConfig) { q.QuotaMediaMB = 1 })))
	c := e.browser()
	c.register("stream-quota@example.com")
	requireQuotaExceeded(t, c.upload("big.png", big), "media_bytes")
	if e.storage.Len() != 0 {
		t.Fatalf("a refused upload left %d objects behind", e.storage.Len())
	}
	if r := c.upload("small.png", noisePNG(t, 1)); r.status != 201 {
		t.Fatalf("a file under the limit: %d %s", r.status, r.body)
	}

	u := newEnv(t, withQuota(unlimited))
	uc := u.browser()
	uc.register("stream-unlimited@example.com")
	if r := uc.upload("big.png", big); r.status != 201 {
		t.Fatalf("unlimited plan: %d %s", r.status, r.body)
	}
}
