import type { CallToolResult, ToolAnnotations } from "@modelcontextprotocol/sdk/types.js";
import type { z, ZodRawShape } from "zod";
import type { SocialOSClient } from "../api-client.js";

export type Scope =
  | "social:read"
  | "posts:read"
  | "posts:write"
  | "posts:schedule"
  | "posts:publish"
  | "posts:delete"
  | "social:disconnect"
  | "social:connect"
  | "media:write"
  | "analytics:read";

export type Risk = "safe" | "low" | "medium" | "sensitive" | "critical";

export interface ToolDef<S extends ZodRawShape = ZodRawShape> {
  name: string;
  title: string;
  scope: Scope;
  risk: Risk;
  /** Human description; the risk label is prepended automatically. */
  description: string;
  inputSchema: S;
  annotations: ToolAnnotations;
  handler: (client: SocialOSClient, args: z.infer<z.ZodObject<S>>) => Promise<unknown>;
}

/** Helper that preserves the schema type for the handler's args. */
export function defineTool<S extends ZodRawShape>(def: ToolDef<S>): ToolDef {
  return def as unknown as ToolDef;
}

export function jsonResult(data: unknown): CallToolResult {
  return { content: [{ type: "text", text: JSON.stringify(data, null, 2) }] };
}

export class ConfirmationRequired extends Error {}

export function requireConfirm(confirm: boolean | undefined, action: string): void {
  if (confirm !== true) {
    throw new ConfirmationRequired(
      `CONFIRMATION_REQUIRED: ${action} is irreversible or publicly visible. Ask the user for explicit approval, then call again with "confirm": true.`,
    );
  }
}

export const seg = (id: string): string => encodeURIComponent(id);
