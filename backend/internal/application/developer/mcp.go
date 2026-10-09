package developer

import (
	"context"
	"strings"

	"github.com/google/uuid"
	"github.com/socialos/backend/internal/domain/actor"
	"github.com/socialos/backend/internal/domain/approval"
	"github.com/socialos/backend/internal/domain/audit"
	"github.com/socialos/backend/internal/domain/errs"
)

// CreateMCPInput is POST /developer/mcp-connections.
type CreateMCPInput struct {
	Name       string
	ClientName string
	Scopes     []string
}

// MCPCreated is returned once with the raw key and ready-to-paste config.
type MCPCreated struct {
	Connection MCPConnection
	RawKey     string
	Config     map[string]any
}

// CreateMCPConnection creates a dedicated API key + connection record.
func (s *Service) CreateMCPConnection(ctx context.Context, a actor.Actor, in CreateMCPInput) (*MCPCreated, error) {
	if err := a.RequireSession(); err != nil {
		return nil, err
	}
	if err := a.RequireVerified(); err != nil {
		return nil, err
	}
	name, scopes, err := s.validateKeyInput(CreateKeyInput{Name: in.Name, Scopes: in.Scopes})
	if err != nil {
		return nil, err
	}
	client := strings.TrimSpace(in.ClientName)
	if len(client) > 100 {
		return nil, errs.Validationf("client_name too long").WithField("client_name", "max 100 characters")
	}
	var out MCPCreated
	err = s.tx.InTx(ctx, func(ctx context.Context) error {
		key, raw, err := s.insertKey(ctx, a, "MCP: "+name, scopes, nil, approval.PolicyApprove)
		if err != nil {
			return err
		}
		c := MCPConnection{ID: uuid.New(), UserID: a.UserID, APIKeyID: key.ID, Name: name, ClientName: client,
			KeyPrefix: key.Prefix, Scopes: scopes, CreatedAt: s.clock.Now()}
		if err := s.conns.Create(ctx, &c); err != nil {
			return err
		}
		out = MCPCreated{Connection: c, RawKey: raw, Config: s.mcpConfig(raw)}
		return s.audit.Record(ctx, a, audit.ActionMCPCreated, "mcp_connection", c.ID.String(),
			map[string]any{"name": name, "client_name": client, "api_key_id": key.ID.String()})
	})
	if err != nil {
		return nil, err
	}
	return &out, nil
}

// mcpConfig builds ready-to-paste client configuration for the new key.
//   - http: Streamable HTTP clients (Claude Code, Cursor, …)
//   - stdio: local clients that spawn the server (Claude Desktop)
//   - mcpServers: same as http, so the whole object can be pasted as-is
//   - claude_code: one-line `claude mcp add` command
func (s *Service) mcpConfig(raw string) map[string]any {
	http := map[string]any{
		"mcpServers": map[string]any{
			"socialos": map[string]any{
				"type":    "http",
				"url":     s.mcpPublicURL,
				"headers": map[string]string{"Authorization": "Bearer " + raw},
			},
		},
	}
	stdio := map[string]any{
		"mcpServers": map[string]any{
			"socialos": map[string]any{
				"command": "npx",
				"args":    []string{"-y", "socialos-mcp", "--stdio"},
				"env":     map[string]string{"SOCIALOS_API_KEY": raw, "SOCIALOS_API_URL": s.apiPublicURL},
			},
		},
	}
	return map[string]any{
		"http":       http,
		"stdio":      stdio,
		"mcpServers": http["mcpServers"],
		"claude_code": "claude mcp add --transport http socialos " + s.mcpPublicURL +
			` --header "Authorization: Bearer ` + raw + `"`,
	}
}

// ListMCPConnections lists the tenant's connections.
func (s *Service) ListMCPConnections(ctx context.Context, a actor.Actor) ([]MCPConnection, error) {
	if err := a.RequireSession(); err != nil {
		return nil, err
	}
	return s.conns.List(ctx, a.UserID)
}

// RevokeMCPConnection revokes the connection and its key.
func (s *Service) RevokeMCPConnection(ctx context.Context, a actor.Actor, id uuid.UUID) error {
	if err := a.RequireSession(); err != nil {
		return err
	}
	return s.tx.InTx(ctx, func(ctx context.Context) error {
		c, err := s.conns.Get(ctx, a.UserID, id)
		if err != nil {
			return err
		}
		now := s.clock.Now()
		if err := s.conns.Revoke(ctx, a.UserID, id, now); err != nil {
			return err
		}
		if err := s.keys.Revoke(ctx, a.UserID, c.APIKeyID, now); err != nil {
			return err
		}
		return s.audit.Record(ctx, a, audit.ActionMCPRevoked, "mcp_connection", id.String(), nil)
	})
}
