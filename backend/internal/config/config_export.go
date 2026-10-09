package config

import "time"

// MaxExportRetentionDays caps EXPORT_RETENTION_DAYS: a ZIP holds everything a user owns, so it should not linger.
const MaxExportRetentionDays = 30

// MaxDeletionGraceDays caps ACCOUNT_DELETION_GRACE_DAYS.
const MaxDeletionGraceDays = 30

// ExportConfig tunes the owner's data rights: export (D-018) and account deletion (D-019).
type ExportConfig struct {
	// ExportRetentionDays is how long a finished export stays downloadable before the worker deletes it.
	ExportRetentionDays int
	// DeletionGraceDays is how long an account stays recoverable after its owner asked for deletion.
	DeletionGraceDays int
}

func loadExportConfig() ExportConfig {
	return ExportConfig{ExportRetentionDays: envInt("EXPORT_RETENTION_DAYS", 7), DeletionGraceDays: envInt("ACCOUNT_DELETION_GRACE_DAYS", 7)}
}

// ExportRetention is the retention as a duration.
func (c ExportConfig) ExportRetention() time.Duration {
	return time.Duration(c.ExportRetentionDays) * 24 * time.Hour
}

// DeletionGrace is the grace period as a duration.
func (c ExportConfig) DeletionGrace() time.Duration {
	return time.Duration(c.DeletionGraceDays) * 24 * time.Hour
}

func (c *Config) validateExport() []string {
	var p []string
	if c.ExportRetentionDays < 1 || c.ExportRetentionDays > MaxExportRetentionDays {
		p = append(p, "EXPORT_RETENTION_DAYS must be between 1 and 30")
	}
	// 0 would delete an account the moment it is requested, with no way back.
	if c.DeletionGraceDays < 1 || c.DeletionGraceDays > MaxDeletionGraceDays {
		p = append(p, "ACCOUNT_DELETION_GRACE_DAYS must be between 1 and 30")
	}
	return p
}
