import { describe, expect, it } from "vitest";
import { loadConfig, normalizeApiUrl, readEnv } from "../src/config.js";

describe("config", () => {
  it("normalises the API url", () => {
    expect(normalizeApiUrl("http://api:8080/")).toBe("http://api:8080/api/v1");
    expect(normalizeApiUrl("http://api:8080/api/v1/")).toBe("http://api:8080/api/v1");
  });
  it("has sane defaults and validates", () => {
    expect(loadConfig({})).toMatchObject({ port: 3333, apiUrl: "http://localhost:8080/api/v1" });
    expect(() => loadConfig({ PORT: "abc" })).toThrow();
  });

  describe("STEERPOST_* with SOCIALOS_* fallback (D-020)", () => {
    it("reads the new name", () => {
      const c = loadConfig({ STEERPOST_API_URL: "http://new:1", STEERPOST_TIMEOUT_MS: "2000" });
      expect(c).toMatchObject({ apiUrl: "http://new:1/api/v1", timeoutMs: 2000 });
    });
    it("falls back to the old name", () => {
      const c = loadConfig({ SOCIALOS_API_URL: "http://old:1", SOCIALOS_TIMEOUT_MS: "3000" });
      expect(c).toMatchObject({ apiUrl: "http://old:1/api/v1", timeoutMs: 3000 });
    });
    it("prefers the new name when both are set", () => {
      const c = loadConfig({ STEERPOST_API_URL: "http://new:1", SOCIALOS_API_URL: "http://old:1" });
      expect(c.apiUrl).toBe("http://new:1/api/v1");
    });
    it("treats an empty new value as unset", () => {
      expect(readEnv({ STEERPOST_API_KEY: "", SOCIALOS_API_KEY: "k" }, "API_KEY")).toBe("k");
      expect(readEnv({}, "API_KEY")).toBeUndefined();
    });
    it("validates whichever name is used", () => {
      expect(() => loadConfig({ SOCIALOS_TIMEOUT_MS: "-1" })).toThrow(/TIMEOUT_MS/);
      expect(() => loadConfig({ STEERPOST_TIMEOUT_MS: "x" })).toThrow(/TIMEOUT_MS/);
    });
  });
});
