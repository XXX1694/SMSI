# Review: Bahasa Indonesia (id)

One section per surface. The app's catalog state is `REVIEW` in `frontend/src/i18n/locales.ts`; the landing's is `review` in
`site/i18n/locales.mjs`.

## Landing

Status: `machine-draft` (shown as "Beta translation" in the page footer).
Reviewer: none yet. Date: none yet. Catalog: `site/i18n/landing.id.json`, first drafted with the PR that added it.

Register: Anda; established loans (akun, unggah, pos).

To mark the review done, set `review: 'native-reviewed'` for this locale in `site/i18n/locales.mjs` (the page then drops the
"Beta translation" note), and record the reviewer, date and catalog commit above.

### Strings that need a native look

- `hero.l1` «Agen AI menyusun pos»; `mcp.f1` «cakupan» for scope.
- `facts.retries` «Percobaan ulang, lalu galat yang jelas»: «galat» vs «error» (developers often write error).
- Glossary id column was empty: terms are proposals.
- All strings are machine-drafted from `landing.en.json` with `glossary.csv`; none has been back-translated yet. Errors and
  confirmations do not appear on the landing page, so the risk is wording and tone, not meaning.
- Docs and the demo stay English; links to them carry "(EN)".

## App

Status: `machine-draft` (shown as "Beta translation").
Reviewer: none yet. Date: none yet. Catalog: `frontend/messages/id/*.json` (676 keys, drafted from `frontend/messages/en/*.json` and `meta.json`).

Register: Anda, capitalized. Buttons use the base verb (Simpan, Jadwalkan); body text is polite (Hubungkan akun.). Quotes “…”, sentence case, no exclamation marks.

### Terminology

- The glossary `id` column is still empty, so every term is a proposal. Terms follow `landing.id.json`: pos (post), draf (draft), akun (account), jaringan (network), jadwalkan (schedule), publikasikan (publish), persetujuan (approval), kunci API (API key), kunci tepercaya (trusted key), jaringan uji (test network), unggah (upload).
- Gaps filled here: scope -> cakupan; permission -> izin; tool -> alat; connect -> hubungkan, reconnect -> hubungkan ulang, disconnect -> putuskan (koneksi); revoke -> cabut; approve / deny -> Setujui / Tolak; retry -> Ulangi, try again -> Coba lagi (kept apart); attempt -> percobaan; unconfirmed -> belum pasti; capability -> kemampuan; time zone -> zona waktu; media library -> pustaka media; attach -> lampirkan; file -> berkas; error -> galat; week -> pekan (avoids "Minggu" = Sunday); one-time code -> kode sekali pakai; chat -> obrolan; agent -> agen.
- Post statuses use "terbit" (Terbit, Terbit sebagian, Menerbitkan); actions and prose use "publikasikan / memublikasikan". Same split as the landing.
- Kept in English: Developer (page name), Reset, Media, Email, Demo, Edit, server, token, webhook, log, browser, tab, default.

### Plurals

Indonesian has only `other`. Every plural is written as `{count, plural, other {# pos}}` with the counter word in the string (pos, percobaan, permintaan, karakter, berkas, langkah, hari, menit, lampiran). No reduplication. `select` keys (true/other, post and capability codes) are unchanged.

### Strings that need a native look

- `common.status.target.needs_review`, `common.status.attempt.unknown`: «Belum pasti» (length budget; «Belum terkonfirmasi» is clearer but 19 chars).
- `settings.verified` / `settings.notVerified`: «diverifikasi» / «tak diverifikasi» (length budget; «tak» is informal).
- `common.retry` «Ulangi» vs `common.tryAgain` «Coba lagi»; `approvals.action.post_retry_now` «Ulangi sekarang». Check the two stay distinct.
- `accounts.disconnect` «Putuskan» (button) is short but can read as "decide"; the dialog and labels say «Putuskan koneksi».
- `accounts.reconnect` «Hubungkan ulang» is 15 chars (limit 13); kept for consistency with «dihubungkan ulang» in status and errors.
- `composer.publishNow` «Publikasikan sekarang» is 21 chars (limit 16); «sekarang» carries the safety meaning, so it was kept.
- `accounts.telegram.step1`: «Kirim pesan» must match Telegram's own admin-right name in Indonesian.
- `accounts.telegram.yourChat` «obrolan Anda» for chat.
- `calendar.week` «Pekan» vs «Minggu»; `calendar.legend` «Keterangan».
- `posts.noTargets` «Tanpa akun» (English says "targets"; the style guide says target -> account).
- `auth.forgotPassword.send` «Kirim tautan reset» mixes «reset» with «atur ulang» used in titles.
- `common.status.account.error` «Galat» (developers often write "error"; same question as on the landing).
- `common.hideField` «Sembunyikan {label}» is 19 chars (screen-reader name only).
- `developer.mcp.bridgeIntro` «Jembatan» for "Bridge"; `developer.apiKeys.namePlaceholder` «Penerbit CI» (example name).
- `settings.legal` «Legal» as a section heading; `settings.legalText` «Admin server» for "server admin".
