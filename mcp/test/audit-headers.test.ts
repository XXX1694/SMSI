import { afterAll, beforeAll, beforeEach, describe, expect, it } from "vitest";
import { redact, registerSecret } from "../src/errors.js";
import { clientIp, normalizeIp, parseTrustedProxies } from "../src/client-ip.js";
import { loadConfig } from "../src/config.js";
import { connect, startStack, VALID_KEY, type Stack } from "./helpers.js";

const SECRET = "gw-secret-0123456789abcdef0123456789abcdef";

async function call(s: Stack, name: string, args: Record<string, unknown>) {
  const c = await connect(s.mcpUrl);
  await c.callTool({ name, arguments: args });
  await c.close();
}

describe("tool header", () => {
  let s: Stack;
  beforeAll(async () => { s = await startStack(); });
  afterAll(async () => { await s.stop(); });
  beforeEach(() => { s.api.calls.length = 0; s.api.meCalls.length = 0; });

  it("sends X-MCP-Tool with the tool name on every tool call, and not on the key check", async () => {
    await call(s, "list_posts", {});
    await call(s, "get_post", { post_id: "p1" });
    expect(s.api.calls.map((c) => c.headers["x-mcp-tool"])).toEqual(["list_posts", "get_post"]);
    expect(s.api.meCalls.length).toBeGreaterThan(0);
    for (const m of s.api.meCalls) expect(m.headers["x-mcp-tool"]).toBeUndefined();
  });

  it("sends no gateway headers without MCP_GATEWAY_SECRET, whatever the caller forged", async () => {
    const c = await connect(s.mcpUrl);
    await c.close();
    await fetch(s.mcpUrl, {
      method: "POST",
      headers: {
        Authorization: `Bearer ${VALID_KEY}`, "Content-Type": "application/json", Accept: "application/json, text/event-stream",
        "X-SocialOS-Gateway": "forged", "X-SocialOS-Client-IP": "203.0.113.5", "X-MCP-Tool": "forged_tool",
      },
      body: JSON.stringify({ jsonrpc: "2.0", id: 1, method: "tools/call", params: { name: "list_posts", arguments: {} } }),
    });
    const sent = s.api.calls[s.api.calls.length - 1]!;
    expect(sent.headers["x-socialos-gateway"]).toBeUndefined();
    expect(sent.headers["x-socialos-client-ip"]).toBeUndefined();
    expect(sent.headers["x-mcp-tool"]).toBe("list_posts"); // set by the server from the registry, never copied from the caller
  });
});

describe("gateway headers", () => {
  let s: Stack;
  beforeAll(async () => { s = await startStack({ gatewaySecret: SECRET, trustedProxies: parseTrustedProxies(false, "") }); });
  afterAll(async () => { await s.stop(); });

  it("sends the secret and the TCP peer address when configured", async () => {
    await call(s, "list_posts", {});
    const sent = s.api.calls[s.api.calls.length - 1]!;
    expect(sent.headers["x-socialos-gateway"]).toBe(SECRET);
    expect(sent.headers["x-socialos-client-ip"]).toBe("127.0.0.1");
  });
});

describe("gateway headers behind a trusted proxy", () => {
  let s: Stack;
  beforeAll(async () => { s = await startStack({ gatewaySecret: SECRET, trustedProxies: parseTrustedProxies(true, "") }); });
  afterAll(async () => { await s.stop(); });

  it("forwards the right-most address that is not a trusted proxy", async () => {
    const c = await connect(s.mcpUrl);
    await c.close();
    await fetch(s.mcpUrl, {
      method: "POST",
      headers: {
        Authorization: `Bearer ${VALID_KEY}`, "Content-Type": "application/json", Accept: "application/json, text/event-stream",
        "X-Forwarded-For": "1.1.1.1, 203.0.113.9, 10.0.0.4", "X-SocialOS-Client-IP": "6.6.6.6",
      },
      body: JSON.stringify({ jsonrpc: "2.0", id: 1, method: "tools/call", params: { name: "list_posts", arguments: {} } }),
    });
    expect(s.api.calls[s.api.calls.length - 1]!.headers["x-socialos-client-ip"]).toBe("203.0.113.9");
  });
});

describe("client ip", () => {
  const req = (peer: string, xff?: string | string[]) =>
    ({ socket: { remoteAddress: peer }, headers: xff === undefined ? {} : { "x-forwarded-for": xff } }) as Parameters<typeof clientIp>[0];

  it("ignores X-Forwarded-For unless the peer is a trusted proxy", () => {
    const none = parseTrustedProxies(false, "");
    expect(clientIp(req("10.0.0.2", "1.2.3.4"), none)).toBe("10.0.0.2");
    const on = parseTrustedProxies(true, "");
    expect(clientIp(req("198.51.100.3", "1.2.3.4"), on)).toBe("198.51.100.3");
    expect(clientIp(req("10.0.0.2", "1.2.3.4"), on)).toBe("1.2.3.4");
  });

  it("walks right to left, skipping trusted hops and malformed entries", () => {
    const on = parseTrustedProxies(true, "");
    expect(clientIp(req("10.0.0.2", "9.9.9.9, 8.8.8.8, 192.168.1.1, garbage"), on)).toBe("8.8.8.8");
    expect(clientIp(req("10.0.0.2", ["7.7.7.7", "6.6.6.6, 10.1.1.1"]), on)).toBe("6.6.6.6");
    expect(clientIp(req("10.0.0.2", "10.9.9.9, 172.16.0.1"), on)).toBe("10.0.0.2");
    expect(clientIp(req("::ffff:10.0.0.2", "[2001:db8::1]:4711"), on)).toBe("2001:db8::1");
  });

  it("honours an explicit TRUSTED_PROXIES list and rejects bad ones", () => {
    const only = parseTrustedProxies(true, "203.0.113.0/24");
    expect(clientIp(req("10.0.0.2", "1.2.3.4"), only)).toBe("10.0.0.2"); // private range no longer trusted
    expect(clientIp(req("203.0.113.7", "1.2.3.4"), only)).toBe("1.2.3.4");
    expect(() => parseTrustedProxies(true, "0.0.0.0/0")).toThrow(/every address/);
    expect(() => parseTrustedProxies(false, "nonsense")).toThrow(/TRUSTED_PROXIES/);
    expect(normalizeIp("::FFFF:1.2.3.4")).toBe("1.2.3.4");
  });

  it("returns nothing without a peer address", () => {
    expect(clientIp({ socket: {}, headers: {} } as Parameters<typeof clientIp>[0], parseTrustedProxies(true, ""))).toBeUndefined();
  });
});

describe("gateway secret handling", () => {
  it("is validated and redacted", () => {
    expect(() => loadConfig({ MCP_GATEWAY_SECRET: "short" })).toThrow(/32/);
    expect(loadConfig({ MCP_GATEWAY_SECRET: SECRET }).gatewaySecret).toBe(SECRET);
    expect(loadConfig({}).gatewaySecret).toBeUndefined();
    registerSecret(SECRET);
    const out = redact(`upstream said ${SECRET} and sk_live_abc123`);
    expect(out).not.toContain(SECRET);
    expect(out).not.toContain("sk_live_abc123");
  });
});
