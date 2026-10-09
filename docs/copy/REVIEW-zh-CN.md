# Review: landing page, 简体中文 (zh-CN)

Status: `machine-draft` (shown as "Beta translation" in the page footer).
Reviewer: none yet. Date: none yet. Catalog: `site/i18n/landing.zh-CN.json`, first drafted with the PR that added it.

Register: No pronoun where possible; 你 only where unavoidable; a half-width space between Chinese and Latin letters or digits (chosen; the style guide leaves it open).

To mark the review done, change the status to `native-reviewed`, add the reviewer and date, and drop the beta note from the
page (`footer.beta` in the catalog and `isEn` in `site/build.mjs`).

## Strings that need a native look

- `hero.*`: «智能体» for agent (alternative: 代理); «帖子» for post.
- `mcp.risk.critical` «严重» for critical.
- `nav.networks` «社交网络».
- `mcp.setup` deliberately does not name Claude (languages.md: no "works with Claude" on zh-CN pages).
- Glossary zh-CN column was empty: terms are proposals.
- All strings are machine-drafted from `landing.en.json` with `glossary.csv`; none has been back-translated yet. Errors and
  confirmations do not appear on the landing page, so the risk is wording and tone, not meaning.
- Docs and the demo stay English; links to them carry "(EN)".
