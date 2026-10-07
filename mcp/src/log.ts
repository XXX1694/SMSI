import { redact } from "./errors.js";

/** Structured stderr logger (stdout is reserved for the stdio transport). Never pass credentials. */
export function log(level: "info" | "warn" | "error", msg: string, fields: Record<string, unknown> = {}): void {
  const line = JSON.stringify({ ts: new Date().toISOString(), level, msg: redact(msg), ...fields });
  process.stderr.write(line + "\n");
}
