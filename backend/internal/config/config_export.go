package config

import "time"

// MaxExportRetentionDays caps EXPORT_RETENTION_DAYS: a ZIP holds everything a user owns, so it should not linger.
const MaxExportRetentionDays = 30

// ExportConfig tunes the account data export (D-018).
type ExportConfig struct {
	// ExportRetentionDays is how long a finished export stays downloadable before the worker deletes it.
	ExportRetentionDays int
}

func loadExportConfig() ExportConfig {
	return ExportConfig{ExportRetentionDays: envInt("EXPORT_RETENTION_DAYS", 7)}
}

// ExportRetention is the retention as a duration.
func (c ExportConfig) ExportRetention() time.Duration {
	return time.Duration(c.ExportRetentionDays) * 24 * time.Hour
}

func (c *Config) validateExport() []string {
	if c.ExportRetentionDays < 1 || c.ExportRetentionDays > MaxExportRetentionDays {
		return []string{"EXPORT_RETENTION_DAYS must be between 1 and 30"}
	}
	return nil
}
