package config

import "time"

// MaxDeletionGraceDays caps ACCOUNT_DELETION_GRACE_DAYS.
const MaxDeletionGraceDays = 30

// DeletionConfig tunes account deletion (D-019).
type DeletionConfig struct {
	// DeletionGraceDays is how long an account stays recoverable after its owner asked for deletion.
	DeletionGraceDays int
}

func loadDeletionConfig() DeletionConfig {
	return DeletionConfig{DeletionGraceDays: envInt("ACCOUNT_DELETION_GRACE_DAYS", 7)}
}

// DeletionGrace is the grace period as a duration.
func (c DeletionConfig) DeletionGrace() time.Duration {
	return time.Duration(c.DeletionGraceDays) * 24 * time.Hour
}

func (c *Config) validateDeletion() []string {
	// 0 would delete an account the moment it is requested, with no way back.
	if c.DeletionGraceDays < 1 || c.DeletionGraceDays > MaxDeletionGraceDays {
		return []string{"ACCOUNT_DELETION_GRACE_DAYS must be between 1 and 30"}
	}
	return nil
}
