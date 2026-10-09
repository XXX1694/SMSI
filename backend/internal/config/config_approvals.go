package config

import "time"

// ApprovalConfig tunes the owner's approval of dangerous actions by API keys (D-013).
type ApprovalConfig struct {
	// AgentMinScheduleLead is how far ahead an API key may schedule without approval (0 = rule off).
	AgentMinScheduleLead time.Duration
	// ApprovalTTL is how long an approval stays open.
	ApprovalTTL time.Duration
	// ApprovalMaxPending caps open approvals per key.
	ApprovalMaxPending int
	// ApprovalRetention is how long decided and expired approvals are kept.
	ApprovalRetention time.Duration
}

func loadApprovalConfig() ApprovalConfig {
	return ApprovalConfig{
		AgentMinScheduleLead: envDuration("AGENT_MIN_SCHEDULE_LEAD", 5*time.Minute),
		ApprovalTTL:          envDuration("APPROVAL_TTL", 10*time.Minute),
		ApprovalMaxPending:   envInt("APPROVAL_MAX_PENDING", 10),
		ApprovalRetention:    envDuration("APPROVAL_RETENTION", 30*24*time.Hour),
	}
}

func (c *Config) validateApprovals() []string {
	var p []string
	if c.AgentMinScheduleLead < 0 || c.AgentMinScheduleLead > 24*time.Hour {
		p = append(p, "AGENT_MIN_SCHEDULE_LEAD must be between 0 (off) and 24h")
	}
	if c.ApprovalTTL < time.Minute || c.ApprovalTTL > 24*time.Hour {
		p = append(p, "APPROVAL_TTL must be between 1m and 24h")
	}
	if c.ApprovalMaxPending < 1 || c.ApprovalMaxPending > 1000 {
		p = append(p, "APPROVAL_MAX_PENDING must be 1-1000 per key")
	}
	if c.ApprovalRetention < 24*time.Hour {
		p = append(p, "APPROVAL_RETENTION must be at least 24h")
	}
	return p
}
