# Company Brain

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

Cold start (`ColdStartService.runOnce`) mines backfilled history into a draft canon: a full-text prefilter finds decision, pricing, and policy language in new chunks (own watermark in `cold_start_state`, capped tier 3 calls per run), the model drafts candidate entries in batches, word-overlap dedup skips facts the canon already covers, and each accepted draft lands through `canon_create_entry_draft` (budget enforced, provenance to source events) with a linked `origin=cold_start` proposal so the standard approve and reject transactions are the review flow. `reviewQueue` returns pending drafts ordered bedrock first, then pricing, then confidence. Org inference, gap clustering, and wiki import are not built yet.

The drift engine runs inside the serving service as `DriftService.runOnce(tenantId)`. Tier 1 is SQL: each new chunk (per-tenant watermark over `event_chunks`) is compared against active canon statement embeddings written by the Go embedding pipeline, with per-source-type threshold discounts from `drift_tuning` and bot-authored chunks excluded. Tier 2 classifies survivors with `claude-haiku-4-5` (structured outputs, zod-validated, three bounded attempts); confirms bump `last_referenced_at`, unrelated drops. Tier 3 drafts corrections with `claude-opus-4-8` using thread context and corroborating chunks. Hygiene runs dedup and cooldown checks before tier 3 spend: one open proposal per entry and conflicting field with evidence attaching in `drift_evidence`, a 14-day cooldown after rejection unless evidence doubles (then flagged recurring), per-owner weekly budgets that queue overflow, and bedrock contradictions escalated to the admin as strategic. Without `ANTHROPIC_API_KEY` the engine reports itself disabled instead of failing. The tier 2 eval lives in `services/serving/eval` and runs with `pnpm eval:tier2` when a key is present.

The serving API (NestJS, `services/serving`) exposes the four spec tools as REST under `/tools`: `query` (canon-first answers with trust labels, receipts, conflicts, and freshness), `entries/:id`, `conflicts`, and `proposals`. Agents authenticate with hashed API keys resolved by a security-definer lookup, carry per-key domain scopes and token-bucket rate tiers, and every call writes an audit row with the tool, entry ids touched, and trust level served. Every request runs in a `brain_app` transaction with `app.tenant_id` set, so RLS holds end to end. Canon-only for now: no stream fallback or LLM synthesis yet, so the answer is the top cited statement.

Canon governance also lives in the database: `canon_create_entry_draft`, `canon_submit_version`, `canon_approve`, `canon_reject`, `canon_decay_sweep`, and `canon_health` are security-invoker functions executed by `brain_app`, so the approval transaction (version, approvals, pointer flip, proposal resolution, audit row) is atomic for every surface that calls it. Version attributes are validated against per-domain JSON Schemas in `domain_schemas` by a trigger using pg_jsonschema; platform defaults ship for the six spec domains and tenants can override. Approval requirements resolve from `approval_policies` by tier and domain, most specific wins, with role ranking and distinct-approver counting for multi-approver policies.

Stream retrieval is the `stream_search` database function, security invoker so tenant RLS applies inside it. It fuses a vector arm and a full-text arm with reciprocal rank fusion, checks the caller's principal set against chunk ACLs inside the query, over-fetches with doubled `ef_search` up to three iterations when filtering starves recall, and weights scores by per-source-type freshness decay from `retrieval_half_life`.

The ingestion worker connects as `brain_worker`, a separate role that owns the operational queue: full access to `ingestion_queue`, `ingestion_dlq`, and `connector_state` across tenants, read access to `connectors` plus a column grant to flip `status`, insert-only on `events` with the tenant checked by RLS per item, and no read access to anything else. `brain_app` has no grants on the queue tables, so serving code can never see raw queue payloads. Duplicate delivery is absorbed by the idempotency unique constraint; the worker treats a unique violation as a counted no-op rather than using `ON CONFLICT`, which under RLS would require select privileges the worker should not hold.

## Schema deviations from the spec

Three corrections to Part II section 4.1, each preserving the spec's invariants:

1. Postgres requires the partition key inside primary key and unique constraints, so `events` keys are `(id, occurred_at)` and idempotency is `unique (tenant_id, connector_id, external_id, occurred_at)`. `occurred_at` is source-stable per item, so duplicate delivery still collapses. `event_chunks` and `audit_log` carry composite keys for the same reason.
2. `event_chunks` partitions by `created_at`, which the spec references but never defines. The column is added. Rows referencing `events` carry `event_occurred_at` so the foreign key can target the composite key.
3. `canon_versions`, `canon_provenance`, `canon_relations`, and `approvals` carry a denormalized `tenant_id` because section 4.2 mandates RLS on every table and a one-line policy needs the column. Consistent with the spec's own ACL denormalization rationale.

`event_chunks.embedding_model` is also stored per row so re-embedding stays traceable when the module 02 provider changes.
