package e2e

import (
	"testing"

	"github.com/socialos/backend/internal/domain/terms"
)

func TestRegisterRequiresAcceptTerms(t *testing.T) {
	e := newEnv(t, envOpts{})
	anon := e.browser()
	for name, body := range map[string]map[string]any{
		"missing": {"email": "terms@example.com", "password": goodPassword},
		"false":   {"email": "terms@example.com", "password": goodPassword, "accept_terms": false},
	} {
		r := anon.do("POST", "/api/v1/auth/register", body)
		if r.status != 400 || r.errCode(t) != "VALIDATION_ERROR" {
			t.Fatalf("%s: %d %s", name, r.status, r.body)
		}
		fields, _ := r.errBody(t)["fields"].(map[string]any)
		if fields["accept_terms"] == nil {
			t.Fatalf("%s: no field message for accept_terms: %s", name, r.body)
		}
	}
	var n int
	_ = e.app.DB.Pool.QueryRow(t.Context(), `SELECT count(*) FROM users WHERE email = 'terms@example.com'`).Scan(&n)
	if n != 0 {
		t.Fatalf("a rejected registration created %d users", n)
	}

	body := map[string]any{"email": "terms@example.com", "password": goodPassword, "accept_terms": true}
	anon.must("POST", "/api/v1/auth/register", body, 201)
	var version string
	var accepted bool
	if err := e.app.DB.Pool.QueryRow(t.Context(),
		`SELECT terms_version, terms_accepted_at IS NOT NULL FROM users WHERE email = 'terms@example.com'`).Scan(&version, &accepted); err != nil {
		t.Fatal(err)
	}
	if version != terms.CurrentVersion || !accepted {
		t.Fatalf("stored terms: version=%q accepted=%v", version, accepted)
	}
}
