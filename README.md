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
supabase start
supabase migration up
supabase test db
```

`supabase status` prints local URLs and keys. Seed data lives in `supabase/seed.sql` and loads on `supabase db reset` or fresh start.

## Database access model

Application services connect as the `brain_app` role and set `app.tenant_id` per session. Row level security enforces tenant isolation on every table; `events` and `audit_log` are append-only because `brain_app` holds no update or delete grants on them. Cross-tenant isolation is asserted by the pgTAP suite in `supabase/tests`, which blocks merge in CI.

Stream retrieval is the `stream_search` database function, security invoker so tenant RLS applies inside it. It fuses a vector arm and a full-text arm with reciprocal rank fusion, checks the caller's principal set against chunk ACLs inside the query, over-fetches with doubled `ef_search` up to three iterations when filtering starves recall, and weights scores by per-source-type freshness decay from `retrieval_half_life`.

The ingestion worker connects as `brain_worker`, a separate role that owns the operational queue: full access to `ingestion_queue`, `ingestion_dlq`, and `connector_state` across tenants, read access to `connectors` plus a column grant to flip `status`, insert-only on `events` with the tenant checked by RLS per item, and no read access to anything else. `brain_app` has no grants on the queue tables, so serving code can never see raw queue payloads. Duplicate delivery is absorbed by the idempotency unique constraint; the worker treats a unique violation as a counted no-op rather than using `ON CONFLICT`, which under RLS would require select privileges the worker should not hold.

## Schema deviations from the spec

Three corrections to Part II section 4.1, each preserving the spec's invariants:

1. Postgres requires the partition key inside primary key and unique constraints, so `events` keys are `(id, occurred_at)` and idempotency is `unique (tenant_id, connector_id, external_id, occurred_at)`. `occurred_at` is source-stable per item, so duplicate delivery still collapses. `event_chunks` and `audit_log` carry composite keys for the same reason.
2. `event_chunks` partitions by `created_at`, which the spec references but never defines. The column is added. Rows referencing `events` carry `event_occurred_at` so the foreign key can target the composite key.
3. `canon_versions`, `canon_provenance`, `canon_relations`, and `approvals` carry a denormalized `tenant_id` because section 4.2 mandates RLS on every table and a one-line policy needs the column. Consistent with the spec's own ACL denormalization rationale.

`event_chunks.embedding_model` is also stored per row so re-embedding stays traceable when the module 02 provider changes.
