# Usage statistics

The proxy can record one row per upstream request and serve aggregates for the
management panel's **Usage** page: tokens and API-equivalent cost over time,
what each subscription delivered against its price, and breakdowns by project,
model, client key, and more.

## Turning it on

Set the flag in `config.yaml`, or use the "Turn on recording" button on the
Usage page:

```yaml
observability:
  usage:
    usage-statistics-enabled: true
```

The change applies on config reload, with no restart. Recording is off in Home
(cluster) mode, where usage is reported to Home instead.

## What is stored

Rows go to a SQLite database at `<auth-dir>/usage.db`. Each row holds:

- Time, provider, model, endpoint, status code, latency, and time to first token.
- Token counts in mutually exclusive buckets: uncached input, cache read, cache
  write (with the 5 minute and 1 hour split), output, and reasoning.
- The upstream account. OAuth accounts are stored by their email or project;
  API-key credentials are stored masked (`sk-a...0001`).
- The client API key, masked.
- Client IP, user agent, and session id.
- The working directory the coding agent declared, and the project derived
  from it.

No prompt or response content is stored, and no full API key.

Rows are kept until you delete them, also after recording is turned off. To
start over, stop the server and delete `usage.db` together with its `-wal` and
`-shm` files. The database file is readable by its owner only.

## Projects

The proxy never sees a repository name. Coding agents state their working
directory in each request (Claude Code in a system message or its system
prompt, Codex in its environment context), and the project is the last
directory of that path. Only those places are read, never user or assistant
content, and only near the start of the conversation.
Agent worktrees are folded into their repository:

| Working directory | Project |
| --- | --- |
| `~/repos/app` | `app` |
| `~/.t3/worktrees/app/t3-1234` | `app` |
| `~/repos/app/.claude/worktrees/feature` | `app` |
| `~/repos/app/.worktrees/feature` | `app` |
| `~/.codex/worktrees/1a2b/app` | `app` |

A request that declares no working directory inherits the one last seen in its
session for the same client key, and is otherwise listed as unknown.

## Cost

Cost is computed at query time from the stored token counts and a price table,
so it is an estimate of what the same usage would cost at API list prices. It is
not a bill. Because nothing is priced at write time, changing a price re-prices
all history.

- Built-in prices live in `internal/usagestats/default_prices.json`, which
  records its source and retrieval date. Refresh that file when providers
  change their prices.
- A model name matches its exact price first, then the longest priced name it
  extends at a separator, so `claude-opus-4-5-20251101` uses the
  `claude-opus-4-5` price.
- Models without a price are listed on the Usage page, and their tokens are
  left out of the cost. Add or override prices with the page's **Prices** button.
- Service tiers, batch discounts, and long-context surcharges are not modelled.

Subscription value compares that cost with the monthly plan price you enter for
each account, prorated to the days recorded in the selected range. Each account
also lists the models it served, with every model's API-equivalent cost and its
share of the account's total.

## Management API

All endpoints are under `/v8/management/observability/usage` and need the
management key. They answer `409` with `usage_statistics_disabled` while
recording is off.

| Endpoint | Purpose |
| --- | --- |
| `GET /stats` | Token, request, and cost totals grouped by `group_by` |
| `GET /speed` | Latency and first-token percentiles and output tokens per second |
| `GET /settings` | Price overrides and plan prices, plus the built-in price table |
| `PUT /settings` | Replace price overrides and plan prices |

Query parameters for `/stats` and `/speed`:

- `from`, `to`: Unix milliseconds. Default is the last 30 days.
- `utc_offset_minutes`: minutes east of UTC, used to bucket days and hours in
  the viewer's calendar.
- `group_by`: comma-separated dimensions. `/stats` accepts `day`, `hour`,
  `model`, `alias`, `provider`, `account`, `auth_index`, `auth_type`,
  `client_key`, `client_ip`, `user_agent`, `endpoint`, `project`, `session_id`,
  `reasoning_effort`, `service_tier`, `status_code`, `stream`, and `failed`.
  `/speed` accepts one of those, except `day` and `hour`.
- `filter.<dimension>=<value>`: keep only rows with that exact value.
- `limit`: maximum number of groups (default 2000). The largest groups are kept.

Example:

```bash
curl -H "Authorization: Bearer $MANAGEMENT_KEY" \
  "http://127.0.0.1:8317/v8/management/observability/usage/stats?group_by=day,project"
```
