# Groundtruth

The governed knowledge layer for companies deploying AI. Full product and system specification in [specs/company-brain-full-spec.md](specs/company-brain-full-spec.md).

## Layout

| Path | What | Stack |
| --- | --- | --- |
| `apps/web` | Web app surfaces | Next.js App Router, TypeScript, Tailwind |
| `services/serving` | Serving API and MCP interface | NestJS |
| `services/ingestion` | Connector runtime, normalizer, event writer | Go |
| `services/embedding` | Embedding pipeline workers | Go |
| `packages/shared` | Shared domain types and enums | TypeScript |
| `supabase` | Migrations, RLS policies, isolation tests, seed | Postgres, pgvector |

## Local development

Requires Node 22+, pnpm, Go 1.25+, Docker, Supabase CLI.

```sh
pnpm install
bin/dev.sh
```

`bin/dev.sh` starts the Supabase stack, applies migrations, and runs all four services: ingestion and embedding daemons (Go), the serving API with the scheduler on port 3001, and the web app on port 3000. `supabase status` prints local URLs and keys. Seed data lives in `supabase/seed.sql` and loads on a fresh stack. Each service also ships a Dockerfile for the promote-the-artifact deploy flow in spec section 9.

Verification: `supabase test db` (pgTAP suites), `go test -race ./...` per Go service with `TEST_DATABASE_URL`, `pnpm -r lint && pnpm -r typecheck && pnpm -r test`, and `pnpm --filter @company-brain/serving test:e2e` for the HTTP suites.

Key environment variables: `DATABASE_URL` and `PAYLOAD_ROOT` everywhere; `ANTHROPIC_API_KEY`, `DRIFT_TIER2_MODEL`, `DRIFT_TIER3_MODEL`, and `SCHEDULER_ENABLED=1` for the drift engine and synthesis; `SLACK_BOT_TOKEN`, `SLACK_SIGNING_SECRET`, `SLACK_APPROVAL_CHANNEL`, and `SLACK_TENANT_ID` for the Slack approval app; `DEV_TENANT_ID` and `DEV_PERSON_ID` for the web dev-auth shim. Every LLM-dependent feature reports itself disabled rather than failing when its key is absent.

## Database access model

Application services connect as the `brain_app` role and set `app.tenant_id` per session. Row level security enforces tenant isolation on every table; `events` and `audit_log` are append-only because `brain_app` holds no update or delete grants on them. Cross-tenant isolation is asserted by the pgTAP suite in `supabase/tests`, which blocks merge in CI.

Cold start (`ColdStartService.runOnce`) mines backfilled history into a draft canon: a full-text prefilter finds decision, pricing, and policy language in new chunks (own watermark in `cold_start_state`, capped tier 3 calls per run), the model drafts candidate entries in batches, word-overlap dedup skips facts the canon already covers, and each accepted draft lands through `canon_create_entry_draft` (budget enforced, provenance to source events) with a linked `origin=cold_start` proposal so the standard approve and reject transactions are the review flow. `reviewQueue` returns pending drafts ordered bedrock first, then pricing, then confidence. Org inference (`ColdStartService.inferOrg`) and gap clustering (`GapService.runOnce`) are built; wiki import is not.

The drift engine runs inside the serving service as `DriftService.runOnce(tenantId)`. Tier 1 is SQL: each new chunk (per-tenant watermark over `event_chunks`) is compared against active canon statement embeddings written by the Go embedding pipeline, with per-source-type threshold discounts from `drift_tuning` and bot-authored chunks excluded. Tier 2 classifies survivors with `claude-haiku-4-5` (structured outputs, zod-validated, three bounded attempts); confirms bump `last_referenced_at`, unrelated drops. Tier 3 drafts corrections with `claude-opus-4-8` using thread context and corroborating chunks. Hygiene runs dedup and cooldown checks before tier 3 spend: one open proposal per entry and conflicting field with evidence attaching in `drift_evidence`, a 14-day cooldown after rejection unless evidence doubles (then flagged recurring), per-owner weekly budgets that queue overflow, and bedrock contradictions escalated to the admin as strategic. Without `ANTHROPIC_API_KEY` the engine reports itself disabled instead of failing. The tier 2 eval lives in `services/serving/eval` and runs with `pnpm eval:tier2` when a key is present.

The serving API (NestJS, `services/serving`) exposes the four spec tools as REST under `/tools`: `query` (canon-first answers with trust labels, receipts, conflicts, and freshness), `entries/:id`, `conflicts`, and `proposals`. Agents authenticate with hashed API keys resolved by a security-definer lookup, carry per-key domain scopes and token-bucket rate tiers, and every call writes an audit row with the tool, entry ids touched, and trust level served. Every request runs in a `brain_app` transaction with `app.tenant_id` set, so RLS holds end to end. When canon coverage is empty and the caller asks for it, `query` falls back to `stream_search` and labels the answer `stream_only`; an LLM synthesises the final sentence from the cited statements, degrading to the top cited statement when no key is configured or the call fails.

Rate limiting and metering are one database call. `meter_and_take` refills a per-key token bucket in `api_key_buckets` from `rate_tiers`, decrements it, and writes the `metering_events` row only when the call is allowed, so limits and usage counts survive multiple serving replicas. A refused call returns `retry_after` and never bills. Calls that fail after authorisation still meter; deriving billing from `audit_log`, which records every served call inside the request transaction, is the follow-up when the billing loop lands.

Metering bills by principal, not by surface. Third-party agents on the MCP and REST paths meter once per authenticated call (`category = 'tool_call'`). The web chat surface is a first-party agent run by a human, so it follows the same rule the answer contract implies: reading the canon is free, running an agent that acts is billable. Every chat turn writes one `metering_events` row tagged `category = 'agent_run'` with the run's token usage, and `billable` is true only when the turn invoked an action tool (`propose_update`, `create_document`, `create_deck`); a pure question-and-answer turn is recorded `billable = false` so its inference cost is visible as telemetry without taxing human engagement. `metering_cost_daily` aggregates the billable rows by tenant, day, and category with summed tokens, so billing charges by cost rather than by a flat query count. Token usage is captured because a chat run fans out to as many as nine model calls, and a per-turn count would undercharge exactly the heaviest runs.

Canon governance also lives in the database: `canon_create_entry_draft`, `canon_submit_version`, `canon_approve`, `canon_reject`, `canon_decay_sweep`, and `canon_health` are security-invoker functions executed by `brain_app`, so the approval transaction (version, approvals, pointer flip, proposal resolution, audit row) is atomic for every surface that calls it. Version attributes are validated against per-domain JSON Schemas in `domain_schemas` by a trigger using pg_jsonschema; platform defaults ship for the six spec domains and tenants can override. Approval requirements resolve from `approval_policies` by tier and domain, most specific wins, with role ranking and distinct-approver counting for multi-approver policies.

Stream retrieval is the `stream_search` database function, security invoker so tenant RLS applies inside it. It fuses a vector arm and a full-text arm with reciprocal rank fusion, checks the caller's principal set against chunk ACLs inside the query, over-fetches with doubled `ef_search` up to three iterations when filtering starves recall, and weights scores by per-source-type freshness decay from `retrieval_half_life`.

## Evidence visibility

Drift evidence follows one rule: the fact crosses, the excerpt does not. Tier 3 drafts from every chunk in the tenant, so a contradiction raised in a private channel still reaches the entry owner. What that owner sees is filtered. Every human read of `drift_evidence` joins through `acl_admits(chunk.acl, principals)` and renders only chunks the viewer could have opened in the source tool; the rest collapse to a count and a list of source types, never channel names or thread subjects. The `admin` role does not bypass this, per spec section 3.1, which forbids admin override on raw content.

Excerpts are attributable by construction. Tier 3 receives evidence as `<evidence id="eN">` blocks and must return each `supporting_excerpt` tagged with the block it quoted, so every excerpt resolves to a chunk id before it is stored in the audit detail. An excerpt naming an unknown block is dropped rather than stored unattributable. Prompt versions `tier3-v2` and `gap-tier3-v2` mark the change.

The residual risk is deliberate and worth stating: a drafted statement is derived from evidence the owner may not be able to see. Tier 3 is instructed to state the corrected fact in its own words and never to quote evidence into `drafted_statement`, and the queue tells the owner how many sources were withheld so they know their view is partial.

`acl_admits` is the single predicate for new call sites. `stream_search` keeps its inlined literal copy on purpose: its vector arm depends on `(acl->>'scope') = 'tenant'` matching the partial HNSW index `event_chunks_tenant_visible_hnsw_idx`, and replacing it with a function call risks losing that index.

## Ingestion

The ingestion worker connects as `brain_worker`, a separate role that owns the operational queue: full access to `ingestion_queue`, `ingestion_dlq`, and `connector_state` across tenants, read access to `connectors` plus a column grant to flip `status`, insert-only on `events` with the tenant checked by RLS per item, and no read access to anything else. `brain_app` has no grants on the queue tables, so serving code can never see raw queue payloads. Duplicate delivery is absorbed by the idempotency unique constraint; the worker treats a unique violation as a counted no-op rather than using `ON CONFLICT`, which under RLS would require select privileges the worker should not hold.

## Schema deviations from the spec

Three corrections to Part II section 4.1, each preserving the spec's invariants:

1. Postgres requires the partition key inside primary key and unique constraints, so `events` keys are `(id, occurred_at)` and idempotency is `unique (tenant_id, connector_id, external_id, occurred_at)`. `occurred_at` is source-stable per item, so duplicate delivery still collapses. `event_chunks` and `audit_log` carry composite keys for the same reason.
2. `event_chunks` partitions by `created_at`, which the spec references but never defines. The column is added. Rows referencing `events` carry `event_occurred_at` so the foreign key can target the composite key.
3. `canon_versions`, `canon_provenance`, `canon_relations`, and `approvals` carry a denormalized `tenant_id` because section 4.2 mandates RLS on every table and a one-line policy needs the column. Consistent with the spec's own ACL denormalization rationale.

`event_chunks.embedding_model` is also stored per row so re-embedding stays traceable when the module 02 provider changes.
