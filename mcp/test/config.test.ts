import { describe, expect, it } from "vitest";
import { loadConfig, normalizeApiUrl } from "../src/config.js";

describe("config", () => {
  it("normalises the API url", () => {
    expect(normalizeApiUrl("http://api:8080/")).toBe("http://api:8080/api/v1");
    expect(normalizeApiUrl("http://api:8080/api/v1/")).toBe("http://api:8080/api/v1");
  });
  it("has sane defaults and validates", () => {
    expect(loadConfig({})).toMatchObject({ port: 3333, apiUrl: "http://localhost:8080/api/v1" });
    expect(() => loadConfig({ PORT: "abc" })).toThrow();
  });
});
