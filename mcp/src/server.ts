import { McpServer } from "@modelcontextprotocol/sdk/server/mcp.js";
import type { CallToolResult } from "@modelcontextprotocol/sdk/types.js";
import type { SocialOSClient } from "./api-client.js";
import { errorResult, toToolError } from "./errors.js";
import { ALL_TOOLS } from "./tools/index.js";
import { ConfirmationRequired, jsonResult } from "./tools/types.js";

export const SERVER_INFO = { name: "socialos", version: "0.1.0" } as const;

const INSTRUCTIONS =
  "SocialOS lets you draft, schedule and publish social media posts. Prefer create_draft, then schedule_post. " +
  "Never call publish_post, delete_post or disconnect_account without the user's explicit approval for that exact action.";

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
          if (err instanceof ConfirmationRequired) return errorResult(err.message);
          if (err instanceof Error && err.message.startsWith("VALIDATION_ERROR:")) return errorResult(err.message);
          return toToolError(err);
        }
      },
    );
  }
  return server;
}
