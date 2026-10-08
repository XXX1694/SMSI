# Contributing

## Size ceilings (ratchet)

The norm is 250 lines per file and 60 per function. CI enforces looser ceilings that are only ever lowered:

| Where | File | Function | Enforced by |
|---|---|---|---|
| Go (`backend`) | 400 | 80 | `internal/archtest` (`maxFileLines`), `funlen` in `backend/.golangci.yml` |
| Frontend | 300 | 80 | `max-lines`, `max-lines-per-function` in `frontend/.eslintrc.json` |
| `mcp` | 300 | 80 | `mcp/scripts/check-size.mjs` |

Test files are exempt. `make lint` runs all of them.

To lower a ceiling: change the number in the place above, run `make lint`, and for every new violation either split
the code or, as a last resort, add the file to the debt list (`overrides` in `.eslintrc.json`, `DEBT` in
`check-size.mjs`) with the comment `debt: split by <YYYY-MM>`. Never raise a ceiling and never grow the debt list
without a reason in the PR.

Layering is checked by the same Go package: `domain` imports nothing from `application`, `adapters`,
`infrastructure` or `transport`, and `transport` does not import `infrastructure/postgres`.
