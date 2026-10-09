package config

import (
	"strings"
	"testing"
)

func TestQuotaDefaultsAndUnlimited(t *testing.T) {
	validEnv(t)
	c, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if l := c.QuotaLimits(); l.Accounts != 5 || l.PostsPerMonth != 60 || l.MediaBytes != 500<<20 {
		t.Fatalf("defaults: %+v", l)
	}
	for _, k := range []string{"QUOTA_ACCOUNTS", "QUOTA_POSTS_PER_MONTH", "QUOTA_MEDIA_MB"} {
		t.Setenv(k, "-1")
	}
	c, err = Load()
	if err != nil {
		t.Fatal(err)
	}
	if l := c.QuotaLimits(); l.Accounts != -1 || l.PostsPerMonth != -1 || l.MediaBytes != -1 {
		t.Fatalf("-1 must stay unlimited: %+v", l)
	}
}

func TestQuotaRejectsZeroAndBelowMinusOne(t *testing.T) {
	for _, v := range []string{"0", "-2"} {
		validEnv(t)
		t.Setenv("QUOTA_MEDIA_MB", v)
		if _, err := Load(); err == nil || !strings.Contains(err.Error(), "QUOTA_MEDIA_MB") {
			t.Errorf("QUOTA_MEDIA_MB=%s: err=%v", v, err)
		}
	}
}
