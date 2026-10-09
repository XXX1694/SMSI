import { McpServer } from "@modelcontextprotocol/sdk/server/mcp.js";
import type { CallToolResult } from "@modelcontextprotocol/sdk/types.js";
import type { SocialOSClient } from "./api-client.js";
import { errorResult, toToolError } from "./errors.js";
import { ALL_TOOLS } from "./tools/index.js";
import { jsonResult } from "./tools/types.js";

export const SERVER_INFO = { name: "steerpost", version: "0.1.0" } as const;

const INSTRUCTIONS =
  "Steerpost lets you draft, schedule and publish social media posts. Prefer create_draft, then schedule_post. Before schedule_post, show the user the final text, accounts and time; scheduled posts publish without another check. " +
  "publish_post, delete_post, disconnect_account and scheduling or editing a post closer than the server's minimum lead (default 5 minutes) need the owner's approval in Steerpost, unless the key is trusted: " +
  "the first call answers APPROVAL_REQUIRED and does nothing. Tell the owner to approve it at the approve_url, wait until they confirm, " +
  "then repeat the identical call with the approval_id. An approval works once and only for that exact call. " +
  "The plan has limits (connected accounts, posts per month, media storage, requests per minute): get_usage shows them if your key has the analytics:read scope, and an action over a limit fails with QUOTA_EXCEEDED and is not performed.";

/** Builds a server exposing only the tools whose scope the API key holds. */
export function buildServer(client: SocialOSClient, scopes: readonly string[]): McpServer {
  const server = new McpServer(SERVER_INFO, { instructions: INSTRUCTIONS });
  const granted = new Set(scopes);

  for (const tool of ALL_TOOLS) {
    if (!granted.has(tool.scope)) continue;
    server.registerTool(
      tool.name,
      {
        title: tool.title,
        description: `[risk: ${tool.risk}; scope: ${tool.scope}] ${tool.description}`,
        inputSchema: tool.inputSchema,
        annotations: tool.annotations,
      },
      async (args: unknown): Promise<CallToolResult> => {
        try {
          return jsonResult(await tool.handler(client.withTool(tool.name), args as never));
        } catch (err) {
          if (err instanceof Error && err.message.startsWith("VALIDATION_ERROR:")) return errorResult(err.message);
          return toToolError(err);
        }
      },
    );
  }
  return server;
}
