# Security

How to report a vulnerability: use GitHub's private vulnerability reporting on this repository (Security → Report a
vulnerability). Please do not open a public issue.

The baseline every change follows is OWASP ASVS L1 (see `AGENTS.md`, section 8). A threat model will be added here.

## Accepted findings

A scanner finding that is not fixed is accepted here with a reason and a date, as `AGENTS.md` requires.

| Date | Finding | Where | Why it is accepted | Revisit |
|---|---|---|---|---|
| 2026-10-09 | KaTeX prototype pollution can bypass trust restrictions ([GHSA-238p-pmpm-9mq7](https://github.com/advisories/GHSA-238p-pmpm-9mq7)), low; reported twice by `npm audit` (katex and mermaid, which depends on it) | `site/` build only: mermaid renders the diagrams in the documentation pages | Low severity. It needs an existing prototype pollution to be exploitable, and the docs render only diagrams from this repository, never user input. The fix `npm audit` offers downgrades mermaid to 10.8.0 (breaking). | When mermaid ships a fixed KaTeX |
| 2026-10-09 | Server-side request to a user-supplied URL (CodeQL `go/request-forgery`, alert 8) | `backend/internal/adapters/bluesky/client.go` | False positive. The base URL is the fixed `https://bsky.social` or a PDS URL that `baseFor` validates (https, port 443, no userinfo, path or query), and requests go through the `safehttp` client, which resolves the host and blocks private, loopback and link-local addresses in its dialer (D-010). CodeQL does not model the dialer guard. | When the adapter changes |
