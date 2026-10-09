package middleware

import (
	"context"
	"log/slog"
	"net/http"
	"regexp"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/cors"

	"github.com/socialos/backend/internal/application/port"
	"github.com/socialos/backend/internal/domain/actor"
	"github.com/socialos/backend/internal/domain/approval"
	"github.com/socialos/backend/internal/domain/audit"
	"github.com/socialos/backend/internal/domain/errs"
	"github.com/socialos/backend/internal/transport/httpx"
)

// toolNameRe is the shape of an MCP tool name accepted from X-MCP-Tool.
var toolNameRe = regexp.MustCompile(`^[a-z_]{1,64}$`)

// idParamRe bounds the URL parameter values copied into the audit log.
var idParamRe = regexp.MustCompile(`^[0-9a-fA-F-]{36}$`)

// APIKeyAudit writes one audit entry for every request made with an API key. A request that carries a valid
// X-MCP-Tool header (the MCP server sets it on every tool call) is recorded as mcp.tool_call instead of
// api_key.request, so a tool call never produces two rows. The tool name is descriptive, not a security input: any
// holder of the key may set it, so an invalid value is dropped and the request is recorded as a plain API request.
//
// The metadata is an allow-list: tool, method, route pattern (never the raw path), status, error code, URL ids,
// the key's label and id (never the key) and whether the address came from the MCP gateway. Headers, query strings
// and bodies never reach it.
func APIKeyAudit(rec port.AuditRecorder, log *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			sr := &statusRecorder{ResponseWriter: w}
			ctx, codes := httpx.WithCodeSlot(r.Context())
			// Written in a defer so that a panicking handler is still audited (as a 500); the panic goes on to Recover.
			defer func() {
				p := recover()
				if p != nil {
					sr.status = http.StatusInternalServerError
					codes.Set(errs.Internal)
				}
				writeAudit(rec, log, r, sr, codes)
				if p != nil {
					panic(p)
				}
			}()
			next.ServeHTTP(sr, r.WithContext(ctx))
		})
	}
}

func writeAudit(rec port.AuditRecorder, log *slog.Logger, r *http.Request, sr *statusRecorder, codes *httpx.CodeSlot) {
	a, ok := actor.From(r.Context())
	if !ok || a.Type != actor.TypeAPIKey {
		return
	}
	if sr.status == 0 {
		sr.status = http.StatusOK
	}
	meta := map[string]any{"method": r.Method, "route": routePattern(r), "status": sr.status}
	action := audit.ActionAPIRequest
	if tool := r.Header.Get(ToolHeader); toolNameRe.MatchString(tool) {
		action = audit.ActionMCPToolCall
		meta["tool"] = tool
		meta["client"] = a.Label
		meta["credential_id"] = a.APIKeyID.String()
		_, meta["via_gateway"] = gatewayIP(r.Context())
		if id, ok := approval.IDFrom(r.Context()); ok {
			meta["approval_id"] = id.String()
		}
		if c := codes.Code(); c != "" {
			meta["error_code"] = string(c)
		}
		if ids := targetIDs(r); len(ids) > 0 {
			meta["target_ids"] = ids
		}
	}
	if err := rec.Record(context.WithoutCancel(r.Context()), a, action, "api_key", a.APIKeyID.String(), meta); err != nil {
		log.WarnContext(r.Context(), "api key audit failed", slog.Any("error", err))
	}
}

// targetIDs returns the id-like URL parameters of the matched route (`id`, `*_id`) whose value is a UUID.
func targetIDs(r *http.Request) map[string]string {
	rc := chi.RouteContext(r.Context())
	if rc == nil {
		return nil
	}
	out := map[string]string{}
	for i, k := range rc.URLParams.Keys {
		if i >= len(rc.URLParams.Values) || (k != "id" && !strings.HasSuffix(k, "_id")) {
			continue
		}
		if v := rc.URLParams.Values[i]; idParamRe.MatchString(v) {
			out[k] = v
		}
	}
	return out
}

// CORS allows credentialed requests only from the configured origins.
func CORS(origins []string) func(http.Handler) http.Handler {
	var allowed []string
	for _, o := range origins {
		if o = strings.TrimSpace(o); o != "" && o != "*" { // never a wildcard together with credentials
			allowed = append(allowed, o)
		}
	}
	if len(allowed) == 0 { // go-chi/cors would otherwise default to "*"
		return func(next http.Handler) http.Handler { return next }
	}
	return cors.Handler(cors.Options{
		AllowedOrigins:   allowed,
		AllowedMethods:   []string{"GET", "POST", "PATCH", "DELETE", "OPTIONS"},
		AllowedHeaders:   []string{"Authorization", "Content-Type", CSRFHeader, "X-Request-ID", "X-Correlation-ID"},
		ExposedHeaders:   []string{"X-Request-ID", "Retry-After"},
		AllowCredentials: true,
		MaxAge:           600,
	})
}
