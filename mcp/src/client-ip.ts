import { BlockList, isIP } from "node:net";
import type { IncomingMessage } from "node:http";

/** Proxy networks whose X-Forwarded-For hops are believed. Empty = nobody (the TCP peer is the client). */
export type TrustedProxies = BlockList;

const DEFAULTS = ["127.0.0.0/8", "10.0.0.0/8", "172.16.0.0/12", "192.168.0.0/16"];
const DEFAULTS_V6 = ["::1/128", "fc00::/7"];

function add(list: BlockList, cidr: string): void {
  const [addr = "", bits] = cidr.split("/");
  const family = isIP(addr);
  if (family === 0) throw new Error(`TRUSTED_PROXIES: "${cidr}" is not a CIDR (e.g. 10.0.0.0/8) or an IP address`);
  const max = family === 4 ? 32 : 128;
  const prefix = bits === undefined ? max : Number(bits);
  if (!Number.isInteger(prefix) || prefix < 0 || prefix > max || (bits !== undefined && bits.trim() === "")) {
    throw new Error(`TRUSTED_PROXIES: "${cidr}" is not a CIDR (e.g. 10.0.0.0/8) or an IP address`);
  }
  if (prefix === 0) {
    throw new Error(`TRUSTED_PROXIES: "${cidr}" would trust every address; list your proxies, or set TRUST_PROXY=false`);
  }
  if (bits === undefined) list.addAddress(addr, family === 4 ? "ipv4" : "ipv6");
  else list.addSubnet(addr, prefix, family === 4 ? "ipv4" : "ipv6");
}

/**
 * Same switch as the backend: TRUST_PROXY off trusts nobody (X-Forwarded-For is never read); on, an empty
 * TRUSTED_PROXIES means loopback + private ranges. The list is parsed either way so a typo fails at startup.
 */
export function parseTrustedProxies(trustProxy: boolean, raw: string): TrustedProxies {
  const parsed = new BlockList();
  let n = 0;
  for (const part of raw.split(",")) {
    const p = part.trim();
    if (p === "") continue;
    add(parsed, p);
    n++;
  }
  if (!trustProxy) return new BlockList();
  if (n > 0) return parsed;
  const out = new BlockList();
  for (const c of [...DEFAULTS, ...DEFAULTS_V6]) add(out, c);
  return out;
}

/** Gives every spelling of an address one form: ::ffff:10.0.0.1 is 10.0.0.1; zone ids and ports are dropped. */
export function normalizeIp(s: string): string | undefined {
  let v = s.trim();
  const bracket = /^\[([^\]]+)\](?::\d+)?$/.exec(v);
  if (bracket) v = bracket[1]!;
  else if (/^\d{1,3}(\.\d{1,3}){3}:\d+$/.test(v)) v = v.slice(0, v.lastIndexOf(":"));
  v = v.replace(/%.*$/, "");
  const mapped = /^::ffff:(\d{1,3}(?:\.\d{1,3}){3})$/i.exec(v);
  if (mapped) v = mapped[1]!;
  return isIP(v) === 0 ? undefined : v.toLowerCase();
}

function trusted(list: TrustedProxies, ip: string): boolean {
  return list.check(ip, isIP(ip) === 4 ? "ipv4" : "ipv6");
}

/**
 * The caller's address, by the backend's rule: start at the TCP peer; when it is not a trusted proxy it is the client
 * and the headers are ignored. Otherwise walk X-Forwarded-For from right to left, skip hops that are trusted proxies
 * and return the first other valid address (the peer when there is none). Malformed hops are skipped.
 * undefined when there is no usable peer address (e.g. a test double).
 */
export function clientIp(req: Pick<IncomingMessage, "socket" | "headers">, list: TrustedProxies): string | undefined {
  const peer = req.socket.remoteAddress ? normalizeIp(req.socket.remoteAddress) : undefined;
  if (!peer) return undefined;
  if (!trusted(list, peer)) return peer;
  const raw = req.headers["x-forwarded-for"];
  const lines = Array.isArray(raw) ? raw : raw === undefined ? [] : [raw];
  for (let i = lines.length - 1; i >= 0; i--) {
    const hops = lines[i]!.split(",");
    for (let j = hops.length - 1; j >= 0; j--) {
      const hop = normalizeIp(hops[j]!);
      if (hop && !trusted(list, hop)) return hop;
    }
  }
  return peer;
}
