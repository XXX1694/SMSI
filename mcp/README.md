# Steerpost MCP server

TypeScript MCP server (official `@modelcontextprotocol/sdk`) that exposes the Steerpost REST API as 14 tools.
It holds no state and no social-network credentials: every call is forwarded to the REST API with the caller's API key.
Tools are listed per key scope (`GET /me`), and the REST API enforces scopes again on every call.

| Env | Default | |
|---|---|---|
| `STEERPOST_API_URL` (legacy `SOCIALOS_API_URL`) | `http://localhost:8080` | REST base (`/api/v1` is appended if missing) |
| `STEERPOST_API_KEY` (legacy `SOCIALOS_API_KEY`) | – | stdio mode only |
| `PORT` / `HOST` | `3333` / `0.0.0.0` | HTTP mode |
| `STEERPOST_TIMEOUT_MS` (legacy `SOCIALOS_TIMEOUT_MS`) | `15000` | upstream timeout |

```bash
npm install && npm run build
npm start                 # Streamable HTTP: POST /mcp, GET /health
npm run start:stdio       # stdio (needs SOCIALOS_API_KEY)
npm run dev               # tsx watch
npm test
docker build -t socialos-local-mcp . && docker run -p 3333:3333 -e SOCIALOS_API_URL=http://api:8080 socialos-local-mcp
```

Dangerous tools (`publish_post`, `delete_post`, `disconnect_account`, and scheduling or editing a post that runs within the server's minimum lead, default 5 minutes) need the owner's approval in Steerpost: the first call answers `APPROVAL_REQUIRED` with an `approval_id` and does nothing, the owner approves under **Approvals**, and the agent repeats the identical call with `approval_id`. Grant their scopes
(`posts:publish`, `posts:delete`, `social:disconnect`) only to keys you trust.

## Client configuration

There is no Steerpost package on npm. Never run an `npx` command for a Steerpost-named package: the name is not ours, and
whoever registers it would receive your API key.

Claude Desktop, option 1: a custom connector (nothing to install). Settings, Customize, Connectors, Add custom connector:
enter the HTTPS MCP URL, choose "No sign-in" and add `Authorization: Bearer sk_live_...` under Request headers
([Anthropic docs](https://claude.com/docs/connectors/custom/remote-mcp)). Request headers are a beta that not every plan has
yet, the server must be reachable from the internet (Claude connects from Anthropic's cloud, so `localhost` does not work), and
OAuth sign-in is not available for Steerpost yet.

Claude Desktop, option 2: the `mcp-remote` bridge (community package, MIT, pinned to an exact version). Edit
`claude_desktop_config.json` and restart Claude Desktop:

```json
{
  "mcpServers": {
    "steerpost": {
      "command": "npx",
      "args": ["-y", "mcp-remote@0.14.3", "http://localhost:3333/mcp", "--header", "Authorization:${SOCIALOS_AUTH_HEADER}"],
      "env": { "SOCIALOS_AUTH_HEADER": "Bearer sk_live_..." }
    }
  }
}
```

The header value lives in `env` so the key is not on the command line and the space in `Bearer ...` survives on Windows.
Upgrade the pinned version deliberately, never use an unpinned `npx -y`.

Claude Desktop, option 3 (local build, stdio):

```json
{
  "mcpServers": {
    "steerpost": {
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
    "steerpost": {
      "type": "http",
      "url": "http://localhost:3333/mcp",
      "headers": { "Authorization": "Bearer sk_live_..." }
    }
  }
}
```

Claude Code: `claude mcp add --transport http steerpost http://localhost:3333/mcp --header "Authorization: Bearer $SOCIALOS_API_KEY"`

Cursor (`mcp.json`): `{ "mcpServers": { "steerpost": { "url": "http://localhost:3333/mcp", "headers": { "Authorization": "Bearer ${env:SOCIALOS_API_KEY}" } } } }`

Other clients without header support can use the same pinned `mcp-remote@0.14.3` bridge as above.
