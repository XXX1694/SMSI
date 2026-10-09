# Steerpost product brief

Written 2026-10-09 against v0.1.0. What to build next and why; status is in [ROADMAP](ROADMAP.md), network facts in
[PLATFORMS](PLATFORMS.md).

## 1. Positioning

**One-liner.** Steerpost is a self-hosted social media scheduler built for AI agents: your agent drafts and schedules
through MCP, and you stay in control.

**Who it is for.**

- Developers, indie makers and small teams who already work with an AI agent and want it to handle routine posting
  without handing it every key.
- People who must run it themselves (privacy, client data rules) or post where big tools do not, such as Telegram.
- Builders who want a scheduling backend for their own agent workflows.

**Jobs to be done.**

1. *When I ship something,* I want my agent to draft a post per network and line them up, *so that* posting takes
   a minute.
2. *When an agent works for me,* I want to limit what it can do and see everything it did, *so that* I can trust it.
3. *When I plan a week,* I want one calendar across networks, *so that* I can see gaps and move posts.
4. *When a post fails,* I want to know why and how to fix it, *so that* it still goes out on time.
5. *When I self-host,* I want one compose file and updates that run themselves.

**Why us.** We are not the only scheduler with MCP, nor the only self-hosted one (section 2), so we never claim "only"
or "first". We compete on things a user can check:

- **Agent safety by design.** A key sees only the tools its scopes allow, risky tools need the owner's approval in the dashboard, and the
  backend audits every tool call with its outcome (D-007).
- **Honest capabilities.** A network that cannot do something says so; nothing returns a fake success.
- **Publishing that never posts twice.** Idempotent targets, safe retries, and `needs_review` after a crash.
- **Small footprint.** In host-proxy mode the long-running containers are capped at about 1 GB of memory in total,
  so Steerpost shares a 2 GB server with another site (D-001).

We lose today on networks (2 live), MCP OAuth, threads, queues, analytics and teams. Section 4 is the plan.

## 2. Competitors

Checked on the vendors' own pages on 2026-10-09; prices are the lowest listed and change often.

| Product | Agent / MCP | Self-host | Networks (Telegram?) | Price from | Open source |
|---|---|---|---|---|---|
| **Steerpost** | Built in, 14 tools, scoped API key; OAuth planned | Yes | 2 live (yes) | Free (self-host) | Licence not chosen |
| [Buffer](https://buffer.com/mcp) | Hosted MCP on every plan, API key | No | 12 (no) | Free 3 channels; $5/channel/month | No |
| [Hootsuite](https://www.hootsuite.com/integrations/mcp) | Hosted MCP, account sign-in | No | 9 (no) | $99/user/month, annual | No |
| [Typefully](https://support.typefully.com/en/articles/13128440-typefully-mcp-server) | Hosted MCP, OAuth or key, on Free | No | 6 (no) | Free 10 posts/month; $10/month | No |
| [Postiz](https://docs.postiz.com/mcp/introduction) | Built in, also when self-hosted; key or OAuth token | Yes | about 34 claimed (yes) | Self-host free; cloud $29/month | AGPL-3.0 |
| [Mixpost](https://docs.mixpost.app/mcp/) | Pro and Enterprise only, bearer token | Only | Lite 3, Pro 11 (no) | Lite free; Pro $299 once | Lite MIT; Pro commercial |
| [Publer](https://publer.com/plans) | MCP with scoped key, Business plan | No | 13 (yes) | Free 3 accounts; $10/account/month for MCP | No |
| [TryPost](https://github.com/trypostit/trypost) | Built in, OAuth 2.1 | Yes | 12 (yes) | Self-host free; cloud $19/month | AGPL-3.0 |
| [Zernio](https://docs.zernio.com/mcp) (was Late) | Hosted MCP, OAuth or key | No | 15 (yes) | Free 2 accounts; $6/account/month | No |
| [Ayrshare](https://www.ayrshare.com/docs/additional/mcp-server) | Hosted MCP, API key | No | 14 (yes) | $149/month | No |

Pricing sources: [Buffer](https://buffer.com/pricing), [Hootsuite](https://www.hootsuite.com/plans),
[Typefully](https://typefully.com/pricing), [Postiz](https://postiz.com/pricing), [Mixpost](https://mixpost.app/pricing),
[TryPost](https://trypost.it/pricing), [Zernio](https://zernio.com/pricing), [Ayrshare](https://www.ayrshare.com/pricing).
Typefully renders prices client-side and TryPost's pages disagree ($19 and $12), so both need a recheck.

**What this means.**

- MCP is table stakes: even free hosted plans (Buffer, Typefully) include it.
- Postiz and TryPost already combine open source, self-hosting, MCP and Telegram, with more networks and MCP OAuth.
  Our case rests on the safety model, honesty and footprint above, not on being first.
- The scheduler products have a calendar, analytics and, mostly, a queue and an AI assistant; many have threads, first
  comments and approvals. Those are the gaps below.

## 3. Feature gap analysis

| Users expect | Steerpost today | Gap |
|---|---|---|
| Many networks | LinkedIn (personal), Telegram | **Critical.** Tier 1 needs no platform review |
| Calendar with drag and drop | Month, week, day; click to open | Drag to reschedule |
| Queue with time slots | Exact time only | Missing |
| Threads, first comment | No | Threads need Bluesky or Mastodon first |
| Carousels, multi-image | LinkedIn 20 images, Telegram albums of 10 | Instagram waits for Meta (Tier 2) |
| Link shortening, UTM | No | UTM is cheap; a shortener needs a domain |
| Media library | Upload, list, delete | Search, tags |
| Analytics, best time to post | Page exists; no network reports metrics | Needs networks with public counts |
| Team workspaces, approvals | One user; agents can be held to drafts by scope | Review inbox now, teams later |
| AI writing assistant | Bring your own agent through MCP | Deliberate: no model costs in the core |
| RSS to post, CSV import | No | Missing; RSS needs the SSRF guard |
| Browser extension, mobile app | Responsive web, checked at 390 px | Later |
| Notifications, webhooks out | Account emails only | Failure alerts missing |
| Templates, hashtags | No | Later; agents cover most of it |
| Data export, account deletion | Tables prepared only | **Required** (AGENTS.md section 7) |

## 4. Prioritised backlog

Score = reach (1–3) · impact (1–3) · confidence (0.5–1) / effort (weeks for one person with agents). "Gate" items ship
before a public announcement whatever their score. Tiers refer to [PLATFORMS](PLATFORMS.md).

### Now (next 2–4 weeks)

Full user stories and acceptance criteria are in the issues.

| # | Item | User story (short) | Done when | Depends on | Score |
|---|---|---|---|---|---|
| [#75](https://github.com/XXX1694/SMSI/issues/75) | Failure notifications | I learn a post failed and how to fix it | One email per failed post after the last attempt; banner in every mail mode | Mail port; real domain for prod email | 3·2·0.9/0.5 = 10.8 |
| [#76](https://github.com/XXX1694/SMSI/issues/76) | Onboarding checklist | I get from sign-up to a scheduled post and a connected agent fast | Checklist driven by API state, only configured networks offered | None | 3·2·0.7/0.5 = 8.4 |
| [#77](https://github.com/XXX1694/SMSI/issues/77) | MCP setup per client | I paste one config into Claude Code, Cursor or Claude Desktop | Copy-ready tabs; no unpublished npm package | None | 2·2·1/0.5 = 8.0 |
| [#79](https://github.com/XXX1694/SMSI/issues/79) | Token connect + Discord | I connect a network without a developer app | PLATFORMS PR0a, PR0b, PR1 and the 5-step adapter acceptance | Tier 1; DECISIONS entries | 3·3·0.9/1.5 = 5.4 |
| [#80](https://github.com/XXX1694/SMSI/issues/80) | Mastodon + Bluesky | One draft reaches the fediverse and Bluesky | PR2, PR3; SSRF guard proven by negative tests | Tier 1; #79 | 3·3·0.8/1.5 = 4.8 |
| [#81](https://github.com/XXX1694/SMSI/issues/81) | Agent review inbox | I approve, edit or discard what agents drafted | Filtered list with agent name, actions audited as me | None | 2·3·0.8/1 = 4.8 |
| [#82](https://github.com/XXX1694/SMSI/issues/82) | Drag-and-drop calendar | I rearrange my week by dragging | Drafts and scheduled posts move; keyboard "Move to…" | None | 2·2·0.9/1 = 3.6 |
| [#78](https://github.com/XXX1694/SMSI/issues/78) | Data export, account deletion | I can leave with my data at any time | ZIP export; password-confirmed deletion; tenant tests | Migration 00003; security review | Gate (1·3·0.9/1.5 = 1.8) |

### Next (after Now, roughly a quarter)

| Item | User story (short) | Done when | Depends on | Score |
|---|---|---|---|---|
| Queue with time slots | I say "queue it" and it takes the next free slot | Slots per account; `add_to_queue` MCP tool | None | 2·3·0.7/2 = 2.1 |
| Server-side approval of risky agent actions | An agent asks; I approve in the inbox | Publish, delete and disconnect by keys wait for me | #81 | 2·3·0.6/2 = 1.8 |
| UTM tagging | Links carry campaign tags per network | Opt-in per post, previewed | None | 1·1·0.9/0.5 = 1.8 |
| CSV import | I upload a spreadsheet of posts | Every row validated before drafts are created | None | 1·2·0.8/1 = 1.6 |
| Remaining Tier 1 | I reach Slack, Dev.to, VK, Misskey, WordPress, Ghost | Adapter acceptance per network | Tier 1; #79 | 2·2·0.8/2 = 1.6 |
| Threads | My agent posts a thread to Bluesky or Mastodon | Ordered parts; a partial failure is visible | #80 | 2·2·0.7/2 = 1.4 |
| OAuth 2.1 for MCP | I connect an agent without copying a key | Spec-compliant flow; keys still work | Architect and security review | 2·3·0.6/3 = 1.2 |
| Meta for the owner (Threads, Instagram, Facebook Pages), Tumblr | I post to my own accounts through my own app | OAuth on the HTTPS callback; review needs stated in the UI | Tier 2 | 2·3·0.6/3 = 1.2 |
| Outgoing webhooks | My tools hear when a post publishes or fails | Signed, retried, per-user endpoints | SSRF guard | 1·2·0.8/1.5 = 1.1 |
| Basic analytics | I see likes and reposts per post | Counts for networks that expose them | Tier 1 | 2·2·0.5/2 = 1.0 |

OAuth for MCP scores low only because it is large. Postiz, TryPost and Typefully already offer it, so its design should
start during Now.

### Later

RSS to post (SSRF guard), first comment, best time to post (needs analytics), team workspaces and roles, templates and
hashtag groups, browser extension, native mobile app (a Flutter client is an idea in ROADMAP), and Tier 3 networks (X,
YouTube, TikTok, Reddit, Pinterest, LinkedIn pages) once their review or pricing is accepted by the owner.

## 5. Packaging

**Free self-host.** Everything in this repository, on your own server, with your own network apps. No feature is held
back for a paid tier. **The licence is not chosen yet**: until there is a LICENSE file, the code is public but not open
source, and we must not call it that.

**A possible hosted plan later**, for people who do not want to run servers. No pricing is promised. It would need a
real domain and mail, Meta and LinkedIn app reviews for many users (Tier 3), per-user quotas (prepared in migration 00003),
off-site backups, Terms and Privacy, and support.

**Before announcing publicly:**

- [ ] Licence chosen and a LICENSE file added (owner's decision).
- [ ] SECURITY.md and GitHub private vulnerability reporting enabled; threat model and ASVS L1 review done (ROADMAP 6).
- [ ] Data export and account deletion (#78); Terms and Privacy pages.
- [ ] At least Discord, Mastodon and Bluesky live (#79, #80), so the product is useful without developer apps.
- [ ] MCP setup without the unpublished npm package (#77), or the MCP package published to npm with the owner's "yes".
- [ ] A real domain, production mail on, and off-site backups switched on with a restore drill passed.
- [ ] The acceptance scenario passed in production with real networks.
- [ ] The owner's "yes" for every announcement (AGENTS.md section 9).
