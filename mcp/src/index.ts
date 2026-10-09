#!/usr/bin/env node
import { StdioServerTransport } from "@modelcontextprotocol/sdk/server/stdio.js";
import { SocialOSClient } from "./api-client.js";
import { loadConfig, readEnv } from "./config.js";
import { createHttpServer } from "./http.js";
import { log } from "./log.js";
import { buildServer } from "./server.js";

async function runStdio(): Promise<void> {
  const config = loadConfig();
  const apiKey = readEnv(process.env, "API_KEY");
  if (!apiKey) throw new Error("STEERPOST_API_KEY (or the legacy SOCIALOS_API_KEY) is required in stdio mode");
  const client = new SocialOSClient({ baseUrl: config.apiUrl, apiKey, timeoutMs: config.timeoutMs });
  const { scopes } = await client.me();
  await buildServer(client, scopes).connect(new StdioServerTransport());
  log("info", "socialos-mcp stdio ready", { tools_scopes: scopes.length });
}

function runHttp(): void {
  const config = loadConfig();
  const server = createHttpServer(config);
  server.listen(config.port, config.host, () => log("info", "socialos-mcp listening", { port: config.port, api: config.apiUrl }));
  const stop = (): void => {
    server.close(() => process.exit(0));
    setTimeout(() => process.exit(0), 5000).unref();
  };
  process.on("SIGTERM", stop);
  process.on("SIGINT", stop);
}

const stdio = process.argv.includes("--stdio") || process.env.MCP_TRANSPORT === "stdio";
try {
  if (stdio) await runStdio();
  else runHttp();
} catch (err) {
  log("error", "startup failed", { error: err instanceof Error ? err.message : "unknown" });
  process.exit(1);
}
