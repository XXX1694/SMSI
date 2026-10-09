# Review: 简体中文 (zh-CN)

One section per surface. The app's catalog state is `REVIEW` in `frontend/src/i18n/locales.ts`; the landing's is `review` in
`site/i18n/locales.mjs`.

## Landing

Status: `machine-draft` (shown as "Beta translation" in the page footer).
Reviewer: none yet. Date: none yet. Catalog: `site/i18n/landing.zh-CN.json`, first drafted with the PR that added it.

Register: No pronoun where possible; 你 only where unavoidable; a half-width space between Chinese and Latin letters or digits (chosen; the style guide leaves it open).

To mark the review done, set `review: 'native-reviewed'` for this locale in `site/i18n/locales.mjs` (the page then drops the
"Beta translation" note), and record the reviewer, date and catalog commit above.

### Strings that need a native look

- `hero.*`: «智能体» for agent (alternative: 代理); «帖子» for post.
- `mcp.risk.critical` «严重» for critical.
- `nav.networks` «社交网络».
- `mcp.setup` deliberately does not name Claude (languages.md: no "works with Claude" on zh-CN pages).
- Glossary zh-CN column was empty: terms are proposals.
- All strings are machine-drafted from `landing.en.json` with `glossary.csv`; none has been back-translated yet. Errors and
  confirmations do not appear on the landing page, so the risk is wording and tone, not meaning.
- Docs and the demo stay English; links to them carry "(EN)".

## App

Status: `machine-draft` (shown as "Beta translation" in the language switcher once `zh-CN` is enabled in `frontend/src/i18n/locales.ts`).
Reviewer: none yet. Date: none yet. Catalog: `frontend/messages/zh-CN.json`, 676 keys, first drafted with the PR that added it.
Checked: `npm run i18n:check` with `zh-CN` enabled locally (0 errors), the demo at 320, 390 and 1440 px on the dashboard, compose,
calendar and settings pages (no horizontal overflow). No back-translation sample yet.

Register: no pronoun where possible, 你 only where unavoidable (errors, hints, onboarding and the audit actor 你); full-width punctuation
(，。：；？（）); curly quotes “…” for names and page names; a half-width space between Chinese and Latin letters or digits, as on the
landing page (`10 MB`, `API 密钥`, `# 个字符`), but none next to a placeholder that holds Chinese text (a label, a date). No `!`. Plurals use `other` only.

### Terminology (follows `docs/copy/glossary.csv`, zh-CN column, and the landing page)

- post 帖子; draft 草稿; schedule 定时发布 (status 已定时); publish 发布, "Publish now" 立即发布.
- account 账号; network 社交网络 (short 网络 where the layout is tight: 所有网络, 由网络定时); connect 连接; reconnect 重新连接; disconnect 断开连接.
- agent 智能体; scope 权限范围; permission 权限; API key API 密钥; revoke 撤销; token 令牌.
- Approvals page 审批 (nav, title, "in Approvals"); approve 批准; deny 拒绝 (glossary had no value); trusted key 受信任的密钥; dangerous 危险; unconfirmed 未确认.
- Try again 重新加载, Retry 重试 (two different keys); Compose 撰写; Developer 开发者; Analytics 数据分析; Media 媒体; audit log 审计日志.
- Network names, `MCP`, `API`, `HTTPS`, `Webhook`, `Claude Code`, `Cursor`, `Claude Desktop` stay Latin.
- No "works with Claude" claim (languages.md): `dashboard.onboarding.steps.agent.hint` says 智能体 instead of naming Claude. The MCP client tabs and the placeholder
  `Claude Desktop` in `developer.mcp.*` are client names the user must recognise to paste a config, so they stay; check whether that is acceptable for this market.

### Strings that need a native look

- `common.never` 「从未」, `developer.apiKeys.expiryNever` 「永不过期」.
- `common.tryAgain` 「重新加载」 vs `common.retry` 「重试」: the two are close in Chinese.
- 审批 (page) vs 批准 (action) vs 待处理/等你处理 (waiting): confirm the family reads as one concept.
- `approvals.status.pending` 「等待中」 vs `common.status.target.pending` 「待发布」.
- `approvals.action.post_schedule_soon` 「定时在几分钟内发布」 (the label is also used inside sentences).
- `accounts.telegram.step1` 「发布消息」: check against Telegram's Simplified Chinese admin-right label.
- `posts.noTargets` 「无发布账号」; `analytics.metrics.*` 曝光量, 互动反应, 链接点击.
- `errors.withRef` 「请求 ID」 for the support reference.
- `common.listSeparator` 「、」.
- `developer.apiKeys.namePlaceholder` 「CI 发布器」.
- `settings.usage.summary` uses an en dash for the date range as in English; `{start} – {end}`.
- `shell.demo.banner` keeps the trailing 「·」 that separates it from the Reset button.
