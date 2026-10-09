package middleware

import (
	"context"
	"net/http"
	"net/netip"
	"strings"

	"github.com/google/uuid"

	"github.com/socialos/backend/internal/domain/approval"
	"github.com/socialos/backend/internal/infrastructure/crypto"
)

// Headers the MCP server adds when it forwards a tool call to the API.
const (
	// GatewayHeader carries MCP_GATEWAY_SECRET.
	GatewayHeader = "X-SocialOS-Gateway"
	// ClientIPHeader carries the end client's address as the MCP server saw it; honoured only with a valid GatewayHeader.
	ClientIPHeader = "X-SocialOS-Client-IP"
	// ToolHeader names the MCP tool behind the request (descriptive only, never a security decision).
	ToolHeader = "X-MCP-Tool"
)

type gatewayIPKey struct{}

// Gateway trusts X-SocialOS-Client-IP only when X-SocialOS-Gateway equals secret (constant-time compare). An empty
// secret never trusts anything, so a deployment without MCP_GATEWAY_SECRET ignores the header entirely. Everything
// else (missing/wrong secret, malformed address) falls back to the normal ClientIP of the request.
func Gateway(secret string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		if secret == "" {
			return next
		}
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if crypto.ConstantTimeEqual(r.Header.Get(GatewayHeader), secret) {
				if ip, err := netip.ParseAddr(strings.TrimSpace(r.Header.Get(ClientIPHeader))); err == nil {
					r = r.WithContext(context.WithValue(r.Context(), gatewayIPKey{}, normalize(ip).String()))
				}
			}
			next.ServeHTTP(w, r)
		})
	}
}

// gatewayIP returns the client address vouched for by the MCP gateway, if any.
func gatewayIP(ctx context.Context) (string, bool) {
	ip, ok := ctx.Value(gatewayIPKey{}).(string)
	return ip, ok
}

// ApprovalHeader carries the id of an approval the owner granted (D-013).
const ApprovalHeader = "X-Approval-Id"

// ApprovalID passes a well-formed X-Approval-Id to the use cases through the context. A malformed value is dropped:
// the call then simply needs a new approval. The id is not a secret and proves nothing alone: the use case checks
// tenant, key, action, target and payload against the stored row.
func ApprovalID(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if id, err := uuid.Parse(r.Header.Get(ApprovalHeader)); err == nil {
			r = r.WithContext(approval.WithID(r.Context(), id))
		}
		next.ServeHTTP(w, r)
	})
}
