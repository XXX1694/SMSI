package config

import (
	"github.com/socialos/backend/internal/domain/quota"
)

// QuotaConfig holds the limits of the single "free" plan (D-014). -1 switches a limit off; self-hosters who do not want
// limits set all four to -1.
type QuotaConfig struct {
	QuotaAccounts      int
	QuotaPostsPerMonth int
	QuotaMediaMB       int
	QuotaAgentRPM      int
}

func loadQuotaConfig() QuotaConfig {
	return QuotaConfig{
		QuotaAccounts:      envInt("QUOTA_ACCOUNTS", 5),
		QuotaPostsPerMonth: envInt("QUOTA_POSTS_PER_MONTH", 60),
		QuotaMediaMB:       envInt("QUOTA_MEDIA_MB", 500),
		QuotaAgentRPM:      envInt("QUOTA_AGENT_RPM", 120),
	}
}

// QuotaLimits converts the settings to domain limits.
func (c QuotaConfig) QuotaLimits() quota.Limits {
	bytes := int64(quota.Unlimited)
	if c.QuotaMediaMB >= 0 {
		bytes = int64(c.QuotaMediaMB) << 20
	}
	return quota.Limits{Accounts: c.QuotaAccounts, PostsPerMonth: c.QuotaPostsPerMonth, MediaBytes: bytes, AgentRPM: c.QuotaAgentRPM}
}

func (c *Config) validateQuota() []string {
	var p []string
	for _, l := range []struct {
		name string
		v    int
	}{{"QUOTA_ACCOUNTS", c.QuotaAccounts}, {"QUOTA_POSTS_PER_MONTH", c.QuotaPostsPerMonth},
		{"QUOTA_MEDIA_MB", c.QuotaMediaMB}, {"QUOTA_AGENT_RPM", c.QuotaAgentRPM}} {
		if l.v < quota.Unlimited || l.v == 0 {
			p = append(p, l.name+" must be -1 (unlimited) or a positive number")
		}
	}
	return p
}
