import type { CallToolResult } from "@modelcontextprotocol/sdk/types.js";
import { ApiError } from "./api-client.js";

const SECRET_PATTERNS: RegExp[] = [/sk_live_[A-Za-z0-9_\-]+/g, /Bearer\s+[A-Za-z0-9._\-~+/=]+/gi];

/** Strips anything that looks like an API key / bearer token. */
export function redact(s: string): string {
  return SECRET_PATTERNS.reduce((acc, re) => acc.replace(re, "[redacted]"), s);
}

const HINTS: Record<string, string> = {
  INSUFFICIENT_SCOPE: "The API key lacks the scope this action needs. Ask the user to grant it in the SocialOS developer portal.",
  FORBIDDEN: "The API key is not allowed to do this.",
  UNAUTHENTICATED: "The API key is invalid, expired or revoked.",
  SOCIAL_ACCOUNT_EXPIRED: "The social account's authorization expired. The user must reconnect it in SocialOS (this cannot be done through the API).",
  INVALID_STATE_TRANSITION: "The post is not in a state that allows this action. Call get_post_status to check its current status.",
  NOT_FOUND: "The resource does not exist or belongs to another user.",
  VALIDATION_ERROR: "The request was rejected as invalid; fix the arguments and retry.",
  RATE_LIMITED: "Rate limited; wait a moment before retrying.",
  PROVIDER_NOT_AVAILABLE: "This social network is not supported yet.",
  PROVIDER_ERROR: "The social network returned an error. It may be temporary.",
  CONFLICT: "The action conflicts with the current state of the resource.",
};

export function errorResult(message: string): CallToolResult {
  return { isError: true, content: [{ type: "text", text: redact(message) }] };
}

/** Maps any thrown value to a safe, descriptive tool error. */
export function toToolError(err: unknown): CallToolResult {
  if (err instanceof ApiError) {
    const hint = HINTS[err.code];
    const parts = [`${err.code}: ${err.message}`];
    if (hint) parts.push(hint);
    if (err.requestId) parts.push(`(request_id: ${err.requestId})`);
    return errorResult(parts.join(" "));
  }
  return errorResult("INTERNAL: unexpected error while calling SocialOS");
}
