#!/usr/bin/env node
// Size ceilings for src/**/*.ts, mirroring frontend/.eslintrc.json: 300 lines per file and 80 per function,
// blank lines and comments not counted. Test files are exempt. Ratchet: lower the numbers, never raise them.
// mcp has no ESLint and TypeScript 7 ships no JS compiler API, so this is a small dependency-free scanner:
// it masks comments/strings, then matches braces. Function detection is a heuristic (`) {`, `): T {`, `=> {`).
// Known debt goes into DEBT with a reason; keep that list empty or shrinking.
import { readdirSync, readFileSync } from "node:fs";
import { join } from "node:path";

const MAX_FILE_LINES = 300;
const MAX_FUNCTION_LINES = 80;
/** debt: split by 2026-11. Relative paths from mcp/. */
const DEBT = new Set([]);
const CONTROL = /\b(if|for|while|switch|catch|with)\s*$/;

function walk(dir) {
  return readdirSync(dir, { withFileTypes: true }).flatMap((e) =>
    e.isDirectory() ? walk(join(dir, e.name)) : [join(dir, e.name)],
  );
}

/** Replaces comments with spaces and string/template bodies with "x", keeping newlines and length. */
function mask(src) {
  let out = "";
  for (let i = 0; i < src.length; ) {
    const c = src[i];
    const two = src.slice(i, i + 2);
    if (two === "//") {
      while (i < src.length && src[i] !== "\n") (out += " "), i++;
    } else if (two === "/*") {
      const end = src.indexOf("*/", i + 2);
      const stop = end === -1 ? src.length : end + 2;
      for (; i < stop; i++) out += src[i] === "\n" ? "\n" : " ";
    } else if (c === '"' || c === "'" || c === "`") {
      out += c;
      i++;
      while (i < src.length && src[i] !== c) {
        if (src[i] === "\\") (out += "x"), i++;
        out += src[i] === "\n" ? "\n" : "x";
        i++;
      }
      out += c;
      i++;
    } else (out += c), i++;
  }
  return out;
}

function analyse(file) {
  const m = mask(readFileSync(file, "utf8"));
  const lines = m.split("\n");
  const isCode = lines.map((l) => l.trim() !== "");
  const total = isCode.filter(Boolean).length;
  const problems = [];
  if (total > MAX_FILE_LINES) problems.push(`${file}: ${total} lines, ceiling ${MAX_FILE_LINES}`);

  const lineOf = (pos) => m.slice(0, pos).split("\n").length - 1;
  const stack = [];
  for (let i = 0; i < m.length; i++) {
    if (m[i] === "{") {
      const before = m.slice(Math.max(0, i - 200), i).trimEnd();
      let fn = false;
      if (before.endsWith("=>")) fn = true;
      else if (/\)\s*(:\s*[^{;=()]+)?$/.test(before)) {
        const head = before.replace(/\)\s*(:\s*[^{;=()]+)?$/, "");
        // Walk back over the balanced parameter list to see what precedes "(".
        fn = !CONTROL.test(head.replace(/\([^()]*$/, "").trimEnd() + " ");
        if (/\b(if|for|while|switch|catch|with)\s*\([^()]*$/.test(head)) fn = false;
      }
      stack.push({ fn, start: lineOf(i) });
    } else if (m[i] === "}") {
      const top = stack.pop();
      if (top?.fn) {
        const end = lineOf(i);
        let n = 0;
        for (let l = top.start; l <= end; l++) if (isCode[l]) n++;
        if (n > MAX_FUNCTION_LINES) problems.push(`${file}:${top.start + 1}: function has ${n} lines, ceiling ${MAX_FUNCTION_LINES}`);
      }
    }
  }
  return problems;
}

const files = walk("src").filter((f) => /\.tsx?$/.test(f) && !/\.test\.tsx?$/.test(f) && !DEBT.has(f));
const errors = files.flatMap(analyse);
if (errors.length > 0) {
  console.error(errors.join("\n"));
  process.exit(1);
}
console.log(`size ceilings ok (${files.length} files)`);
