package emailtoken

import (
	"testing"
	"time"
)

func TestTTLByPurpose(t *testing.T) {
	if VerifyEmail.TTL() != 48*time.Hour || ResetPassword.TTL() != 30*time.Minute {
		t.Fatalf("ttl: %v %v", VerifyEmail.TTL(), ResetPassword.TTL())
	}
	if Cooldown != time.Minute {
		t.Fatalf("cooldown %v", Cooldown)
	}
}
