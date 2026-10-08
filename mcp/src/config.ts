import { parseTrustedProxies, type TrustedProxies } from "./client-ip.js";

export interface Config {
  apiUrl: string;
  port: number;
  host: string;
  timeoutMs: number;
  /** MCP_GATEWAY_SECRET, shared with the backend; when set the API trusts the client IP forwarded by this server. */
  gatewaySecret?: string;
  /** TRUST_PROXY / TRUSTED_PROXIES: proxies whose X-Forwarded-For is believed when working out the client IP. */
  trustedProxies?: TrustedProxies;
}

/** Normalises the REST base so it always ends with `/api/v1` (no trailing slash). */
export function normalizeApiUrl(raw: string): string {
  const trimmed = raw.trim().replace(/\/+$/, "");
  return /\/api\/v1$/.test(trimmed) ? trimmed : `${trimmed}/api/v1`;
}

export function loadConfig(env: NodeJS.ProcessEnv = process.env): Config {
  const timeoutMs = Number(env.SOCIALOS_TIMEOUT_MS ?? 15000);
  const port = Number(env.PORT ?? 3333);
  if (!Number.isFinite(timeoutMs) || timeoutMs <= 0) throw new Error("SOCIALOS_TIMEOUT_MS must be a positive number");
  if (!Number.isInteger(port) || port < 0 || port > 65535) throw new Error("PORT must be a valid port");
  const gatewaySecret = env.MCP_GATEWAY_SECRET?.trim() || undefined;
  if (gatewaySecret !== undefined && gatewaySecret.length < 32) throw new Error("MCP_GATEWAY_SECRET must be at least 32 characters");
  const trust = ["1", "true", "yes", "on"].includes((env.TRUST_PROXY ?? "").trim().toLowerCase());
  return {
    apiUrl: normalizeApiUrl(env.SOCIALOS_API_URL ?? "http://localhost:8080"),
    port,
    host: env.HOST ?? "0.0.0.0",
    timeoutMs,
    gatewaySecret,
    trustedProxies: parseTrustedProxies(trust, env.TRUSTED_PROXIES ?? ""),
  };
}
