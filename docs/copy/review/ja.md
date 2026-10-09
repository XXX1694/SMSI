# Review: 日本語 (ja)

One section per surface. The app's catalog state is `REVIEW` in `frontend/src/i18n/locales.ts`; the landing's is `review` in
`site/i18n/locales.mjs`.

## Landing

Status: `machine-draft` (shown as "Beta translation" in the page footer).
Reviewer: none yet. Date: none yet. Catalog: `site/i18n/landing.ja.json`, first drafted with the PR that added it.

Register: です/ます, no あなた, full-width punctuation, no space between Japanese and Latin (JTF).

To mark the review done, set `review: 'native-reviewed'` for this locale in `site/i18n/locales.mjs` (the page then drops the
"Beta translation" note), and record the reviewer, date and catalog commit above.

### Strings that need a native look

- `hero.l2` 「主導権は常に手元に。」: pronoun-free rendering of "You stay in control".
- `route.gate` 「人が承認」 (avoids あなた).
- `nav.networks` 「対応SNS」 (glossary: SNS).
- Phrase breaks: `how.title`, `mcp.title`, `net.title` use U+200B between phrases for the headline reveal.
- Glossary ja terms follow the existing column.
- All strings are machine-drafted from `landing.en.json` with `glossary.csv`; none has been back-translated yet. Errors and
  confirmations do not appear on the landing page, so the risk is wording and tone, not meaning.
- Docs and the demo stay English; links to them carry "(EN)".

## App

Status: `machine-draft` (shown as "Beta translation" in the language switcher once `ja` is enabled in `frontend/src/i18n/locales.ts`).
Reviewer: none yet. Date: none yet. Catalog: `frontend/messages/ja/*.json`, 676 keys, first drafted with the PR that added it.
Checked: `npm run i18n:check` with `ja` enabled locally (0 errors), the demo at 320, 390 and 1440 px on the dashboard, compose,
calendar and settings pages (no horizontal overflow). No back-translation sample yet.

Register: です/ます for sentences; noun phrases for buttons, labels and tabs; no あなた; full-width 。、：（） and 「」 for page and
button names; half-width Latin letters and digits; no space between Japanese and Latin or digits (JTF), including sizes
(`10MB`). Plurals use `other` only (`#件`, `#文字`, `#分`).

### Terminology (follows `docs/copy/glossary.csv`, ja column)

- post 投稿 (the verb "publish" is 投稿する, "Publish now" 今すぐ投稿; "Schedule" as a button is 予約投稿, as a section 予約); draft 下書き; scheduled 予約済み.
- account アカウント; network SNS; connect 連携 (never 接続, except "MCP connection" MCP接続, "Connect an AI agent" AIエージェントを接続); reconnect 再連携; disconnect 連携を解除.
- agent エージェント; scope スコープ (Developer screens only); permission 権限; API key APIキー; revoke 無効化; token トークン.
- approvals 承認; approve 承認; deny 却下; waiting 保留中; trusted key 信頼済みキー; dangerous 危険; unconfirmed 未確認.
- Try again 再読み込み, Retry 再試行 (two different keys, as in the glossary); Cancel キャンセル; Keep post 投稿を残す.
- Compose 作成; Developer 開発者; Analytics 分析; Media メディア; time zone タイムゾーン; audit log 監査ログ.
- Network names, `MCP`, `API`, `HTTPS`, `JSON`, `Webhook`, `Claude Code`, `Cursor`, `Claude Desktop` stay Latin. The client names in `developer.mcp.*` are product names and stay.

### Strings that need a native look

- `common.never` 「なし」 (shown for "never used" and "never seen"); `未使用` may read better for keys.
- `common.tryAgain` 「再読み込み」: glossary term, but it is also shown after a failed load of a single list.
- `common.listSeparator` 「、」 for lists of network names; `common.status.target.pending` 「待機中」 vs `approvals.status.pending` 「保留中」 (glossary).
- `shell.demo.banner` is shortened to 「データはブラウザー内に保存」 so it fits one line at 320 px.
- `posts.editBlockedOther`, `composer.conflictLocked`: status sentences rebuilt for Japanese word order.
- `posts.noTargets` 「投稿先なし」, `composer.publishTo` 「投稿先」: "target" is not a UI term, 投稿先 is the natural word.
- `accounts.telegram.step1` 「メッセージの投稿」: check against the Telegram Japanese admin-right label.
- `developer.apiKeys.expiryNever` 「なし」, `developer.apiKeys.namePlaceholder` 「CI投稿ツール」.
- `analytics.metrics.*` インプレッション, リアクション, リンクのクリック数 (check against what X, Bluesky and LinkedIn show in Japanese).
- Page names inside sentences use 「」 (「設定」, 「アカウント」ページ, 「投稿」ページ), because the nav labels are short nouns.
- Dates and times come from `Intl` (`2026年10月9日 09:00`). Long dates may break after a number in narrow columns (see the PR).
