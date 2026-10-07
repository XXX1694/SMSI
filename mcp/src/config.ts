export interface Config {
  apiUrl: string;
  port: number;
  host: string;
  timeoutMs: number;
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
  return {
    apiUrl: normalizeApiUrl(env.SOCIALOS_API_URL ?? "http://localhost:8080"),
    port,
    host: env.HOST ?? "0.0.0.0",
    timeoutMs,
  };
}
