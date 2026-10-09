package config

import (
	"strings"
	"testing"
	"time"
)

func TestWorkerSettingsDefaultAndAreValidated(t *testing.T) {
	validEnv(t)
	c, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if c.WorkerShutdownTimeout != 30*time.Second || c.WorkerHTTPAddr != ":8081" {
		t.Fatalf("defaults: %v %q", c.WorkerShutdownTimeout, c.WorkerHTTPAddr)
	}
	t.Setenv("WORKER_SHUTDOWN_TIMEOUT", "35s")
	t.Setenv("WORKER_HTTP_ADDR", "127.0.0.1:9091")
	if c, err = Load(); err != nil || c.WorkerShutdownTimeout != 35*time.Second || c.WorkerHTTPAddr != "127.0.0.1:9091" {
		t.Fatalf("overrides: %+v %v", c, err)
	}
	for _, val := range []string{"1s", "36s", "-5s"} {
		t.Run(val, func(t *testing.T) {
			t.Setenv("WORKER_SHUTDOWN_TIMEOUT", val)
			if _, err := Load(); err == nil || !strings.Contains(err.Error(), "WORKER_SHUTDOWN_TIMEOUT") {
				t.Fatalf("%s should be rejected, got %v", val, err)
			}
		})
	}
}

func TestExportRetentionDefaultsAndIsValidated(t *testing.T) {
	validEnv(t)
	c, err := Load()
	if err != nil || c.ExportRetention() != 7*24*time.Hour {
		t.Fatalf("default: %v %v", c.ExportRetention(), err)
	}
	t.Setenv("EXPORT_RETENTION_DAYS", "30")
	if c, err = Load(); err != nil || c.ExportRetention() != 30*24*time.Hour {
		t.Fatalf("override: %v %v", c.ExportRetention(), err)
	}
	for _, val := range []string{"0", "-1", "31"} {
		t.Setenv("EXPORT_RETENTION_DAYS", val)
		if _, err := Load(); err == nil || !strings.Contains(err.Error(), "EXPORT_RETENTION_DAYS") {
			t.Fatalf("%s should be rejected, got %v", val, err)
		}
	}
}

func TestDeletionGraceDefaultsAndIsValidated(t *testing.T) {
	validEnv(t)
	c, err := Load()
	if err != nil || c.DeletionGrace() != 7*24*time.Hour {
		t.Fatalf("default: %v %v", c.DeletionGrace(), err)
	}
	t.Setenv("ACCOUNT_DELETION_GRACE_DAYS", "14")
	if c, err = Load(); err != nil || c.DeletionGrace() != 14*24*time.Hour {
		t.Fatalf("override: %v %v", c.DeletionGrace(), err)
	}
	// 0 would mean "delete at once, no way back".
	for _, val := range []string{"0", "-1", "31"} {
		t.Setenv("ACCOUNT_DELETION_GRACE_DAYS", val)
		if _, err := Load(); err == nil || !strings.Contains(err.Error(), "ACCOUNT_DELETION_GRACE_DAYS") {
			t.Fatalf("%s should be rejected, got %v", val, err)
		}
	}
}
