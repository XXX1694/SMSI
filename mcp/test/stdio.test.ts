import { afterAll, beforeAll, expect, it } from "vitest";
import { Client } from "@modelcontextprotocol/sdk/client/index.js";
import { StdioClientTransport } from "@modelcontextprotocol/sdk/client/stdio.js";
import { FakeApi, VALID_KEY } from "./helpers.js";

const api = new FakeApi();
beforeAll(() => api.start());
afterAll(() => api.stop());

it("stdio mode uses SOCIALOS_API_KEY and lists scoped tools", async () => {
  api.keys.set(VALID_KEY, ["posts:read"]);
  const transport = new StdioClientTransport({
    command: process.execPath,
    args: ["--import", "tsx", "src/index.ts", "--stdio"],
    env: { PATH: process.env.PATH ?? "", SOCIALOS_API_URL: api.url, SOCIALOS_API_KEY: VALID_KEY },
    stderr: "ignore",
  });
  const client = new Client({ name: "t", version: "0" });
  await client.connect(transport);
  expect((await client.listTools()).tools.map((t) => t.name).sort()).toEqual(["get_post", "get_post_status", "list_posts"]);
  await client.callTool({ name: "get_post", arguments: { post_id: "p9" } });
  expect(api.calls.at(-1)?.path).toBe("/posts/p9");
  expect(api.calls.at(-1)?.headers.authorization).toBe(`Bearer ${VALID_KEY}`);
  await client.close();
});
