// End-to-end acceptance check of the SocialOS MVP flow across API, worker and MCP.
// Requires a running API (SOCIALOS_API_URL, default http://127.0.0.1:8080) with
// SOCIAL_MOCK_PROVIDERS=true, a running worker and a running MCP server (MCP_URL).
//
//   node scripts/acceptance.mjs
import { Client } from "@modelcontextprotocol/sdk/client/index.js";
import { StreamableHTTPClientTransport } from "@modelcontextprotocol/sdk/client/streamableHttp.js";

const API = process.env.SOCIALOS_API_URL ?? "http://127.0.0.1:8080";
const MCP_URL = process.env.MCP_URL ?? "http://127.0.0.1:3333/mcp";

const jar = new Map();
function cookieHeader() {
  return [...jar].map(([k, v]) => `${k}=${v}`).join("; ");
}
function storeCookies(res) {
  for (const c of res.headers.getSetCookie?.() ?? []) {
    const [pair] = c.split(";");
    const i = pair.indexOf("=");
    jar.set(pair.slice(0, i), pair.slice(i + 1));
  }
}
async function browser(method, path, body, csrf) {
  const res = await fetch(new URL(path, API), {
    method,
    redirect: "manual",
    headers: {
      cookie: cookieHeader(),
      ...(body ? { "content-type": "application/json" } : {}),
      ...(csrf ? { "x-csrf-token": csrf } : {}),
    },
    body: body ? JSON.stringify(body) : undefined,
  });
  storeCookies(res);
  return res;
}
function step(msg) {
  console.log(`✓ ${msg}`);
}
function assert(cond, msg) {
  if (!cond) {
    console.error(`✗ ${msg}`);
    process.exit(1);
  }
}

// 1. register
const email = `acceptance+${Date.now()}@example.com`;
let res = await browser("POST", "/api/v1/auth/register", { email, password: "correct horse battery", display_name: "Acceptance" });
assert(res.status === 201 || res.status === 200, `register returned ${res.status}`);
const me = await (await browser("GET", "/api/v1/me")).json();
const csrf = me.csrf_token;
step(`user registered (${email})`);

// 2-3. connect the mock network twice via the OAuth redirect flow (stands in for LinkedIn + Telegram)
for (let i = 0; i < 2; i++) {
  res = await browser("GET", `/api/v1/social/mock/connect?redirect=/accounts&account=${["linkedin", "telegram"][i]}`);
  const authorize = res.headers.get("location");
  assert(authorize, "connect did not redirect");
  res = await browser("GET", authorize.startsWith("http") ? authorize : new URL(authorize, API).toString());
  assert((res.headers.get("location") ?? "").includes("connected=mock"), `callback redirect: ${res.headers.get("location")}`);
}
step("social accounts connected via OAuth flow (mock provider)");

// 9. create MCP connection without publish/delete permissions
res = await browser("POST", "/api/v1/developer/mcp-connections",
  { name: "Acceptance agent", scopes: ["social:read", "posts:read", "posts:write", "posts:schedule", "analytics:read"] }, csrf);
assert(res.status === 201, `mcp connection returned ${res.status}`);
const conn = await res.json();
assert(typeof conn.key === "string" && conn.key.startsWith("sk_live_"), "raw key missing");
step("MCP connection created, raw key shown once");

// 10. AI agent connects
const client = new Client({ name: "acceptance-agent", version: "1.0.0" });
await client.connect(new StreamableHTTPClientTransport(new URL(MCP_URL), {
  requestInit: { headers: { authorization: `Bearer ${conn.key}` } },
}));
const { tools } = await client.listTools();
const names = tools.map((t) => t.name);
assert(names.includes("create_draft") && names.includes("schedule_post"), `tools: ${names}`);
assert(!names.includes("publish_post") && !names.includes("delete_post") && !names.includes("disconnect_account"),
  "dangerous tools must be hidden without their scopes");
step(`agent connected; ${names.length} tools visible, dangerous tools hidden`);

const parse = (r) => JSON.parse(r.content[0].text);

// 11. list_social_accounts
const accounts = parse(await client.callTool({ name: "list_social_accounts", arguments: {} }));
const accountIds = (accounts.items ?? accounts).map((a) => a.id);
assert(accountIds.length === 2, `expected 2 accounts, got ${accountIds.length}`);
step("list_social_accounts returned 2 accounts");

// 12. create_draft
const draft = parse(await client.callTool({ name: "create_draft", arguments: {
  content: "Shipped a new Flutter project today 🚀", social_account_ids: accountIds } }));
assert(draft.status === "draft", `draft status ${draft.status}`);
step(`create_draft → post ${draft.id}`);

// 13. schedule_post a few seconds ahead
const at = new Date(Date.now() + 4000).toISOString();
const scheduled = parse(await client.callTool({ name: "schedule_post", arguments: { post_id: draft.id, scheduled_at: at } }));
assert(scheduled.status === "scheduled", `schedule status ${scheduled.status}`);
step(`schedule_post at ${at}`);

// 14. worker publishes at the right time
let status;
for (let i = 0; i < 30; i++) {
  await new Promise((r) => setTimeout(r, 1000));
  status = parse(await client.callTool({ name: "get_post_status", arguments: { post_id: draft.id } }));
  if (["published", "failed", "partially_published"].includes(status.status)) break;
}
assert(status.status === "published", `final status ${JSON.stringify(status)}`);
const post = (await (await browser("GET", `/api/v1/posts/${draft.id}`)).json());
const publishedAt = new Date(post.published_at ?? post.targets?.[0]?.published_at);
assert(publishedAt >= new Date(at), `published before schedule: ${publishedAt.toISOString()} < ${at}`);
step(`worker published both targets at ${publishedAt.toISOString()}`);

// publish via a key without posts:publish is refused by the API itself
res = await fetch(new URL(`/api/v1/posts/${draft.id}/publish`, API), { method: "POST", headers: { authorization: `Bearer ${conn.key}` } });
const err = await res.json();
assert(res.status === 403 && err.error.code === "INSUFFICIENT_SCOPE", `publish without scope: ${res.status} ${JSON.stringify(err)}`);
step("API rejects publish without posts:publish (403 INSUFFICIENT_SCOPE)");

// 15. user sees the result on the dashboard + audit trail
const summary = await (await browser("GET", "/api/v1/dashboard/summary")).json();
assert(summary.published_this_month >= 1, `summary ${JSON.stringify(summary)}`);
const audit = await (await browser("GET", "/api/v1/audit-logs?limit=50")).json();
const actions = audit.items.map((a) => `${a.actor_type}:${a.action}`);
assert(actions.some((a) => a.startsWith("api_key:")) && actions.some((a) => a.startsWith("scheduler:")), `audit ${actions}`);
step(`dashboard: published_this_month=${summary.published_this_month}; audit has agent + scheduler entries`);

await client.close();
console.log("\nACCEPTANCE FLOW PASSED");
