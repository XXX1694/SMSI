# SocialOS MCP server

TypeScript MCP server (official `@modelcontextprotocol/sdk`) that exposes the SocialOS REST API as 13 tools.
It holds no state and no social-network credentials: every call is forwarded to the REST API with the caller's API key.
Tools are listed per key scope (`GET /me`), and the REST API enforces scopes again on every call.

| Env | Default | |
|---|---|---|
| `SOCIALOS_API_URL` | `http://localhost:8080` | REST base (`/api/v1` is appended if missing) |
| `SOCIALOS_API_KEY` | – | stdio mode only |
| `PORT` / `HOST` | `3333` / `0.0.0.0` | HTTP mode |
| `SOCIALOS_TIMEOUT_MS` | `15000` | upstream timeout |

```bash
npm install && npm run build
npm start                 # Streamable HTTP: POST /mcp, GET /health
npm run start:stdio       # stdio (needs SOCIALOS_API_KEY)
npm run dev               # tsx watch
npm test
docker build -t socialos-mcp . && docker run -p 3333:3333 -e SOCIALOS_API_URL=http://api:8080 socialos-mcp
```

Dangerous tools (`publish_post`, `delete_post`, `disconnect_account`, and scheduling less than 5 minutes ahead) need the owner's approval in SocialOS: the first call answers `APPROVAL_REQUIRED` with an `approval_id` and does nothing, the owner approves under **Approvals**, and the agent repeats the identical call with `approval_id`. Grant their scopes
(`posts:publish`, `posts:delete`, `social:disconnect`) only to keys you trust.

## Client configuration

Claude Desktop (stdio, local build):

```json
{
  "mcpServers": {
    "socialos": {
      "command": "node",
      "args": ["/path/to/SMSI/mcp/dist/index.js", "--stdio"],
      "env": { "SOCIALOS_API_URL": "http://localhost:8080", "SOCIALOS_API_KEY": "sk_live_..." }
    }
  }
}
```

Remote / HTTP (clients that support Streamable HTTP with headers):

```json
{
  "mcpServers": {
    "socialos": {
      "type": "http",
      "url": "http://localhost:3333/mcp",
      "headers": { "Authorization": "Bearer sk_live_..." }
    }
  }
}
```

Claude Code: `claude mcp add --transport http socialos http://localhost:3333/mcp --header "Authorization: Bearer sk_live_..."`

Clients without header support can bridge with `npx mcp-remote http://localhost:3333/mcp --header "Authorization: Bearer sk_live_..."`.
