# Production deploy

Direct from dev to production. Railway for all four services, Supabase for Postgres + pgvector and for encrypted payload storage.

## Topology

| Service | Runtime | Public | Build context | Railway root dir | Config file |
| --- | --- | --- | --- | --- | --- |
| web | Next.js | yes | repo root | `/` | `apps/web/railway.json` |
| serving | NestJS | yes, secret-guarded | repo root | `/` | `services/serving/railway.json` |
| embedding | Go worker | no | `services/embedding` | `services/embedding` | `railway.json` |
| ingestion | Go worker | no until Slack | `services/ingestion` | `services/ingestion` | `railway.json` |

All four share one Supabase database. ingestion and embedding also share one Supabase Storage bucket and one `MASTER_KEY`: ingestion writes encrypted payloads, embedding decrypts them.

## Hard rules

- Migrations are forward-only. Never run reset, drop, or destructive SQL against the production database.
- The database connection MUST use Supabase's session-mode pooler. Every service does `set role brain_app | brain_worker | brain_embedder` after connecting, and transaction pooling drops the role between statements. Session mode or a direct connection only.
- `MASTER_KEY` is the same 32-byte hex value on ingestion and embedding, and it never changes. Lose it and every stored payload is unrecoverable.
- `INTERNAL_API_SECRET` is the same value on web and serving. It is the only thing guarding the serving API on its public URL.
- Never commit any of these secrets. They live in Railway variables only.

## 0. Accounts and CLIs

Accounts: Railway, Supabase, Voyage AI, Anthropic.

```
brew install railway supabase/tap/supabase
railway login
supabase login
```

## 1. Generate secrets

```
openssl rand -hex 32   # MASTER_KEY
openssl rand -hex 32   # INTERNAL_API_SECRET
```

Keep both. You will paste them into Railway.

## 2. Supabase

1. Create a project. Note the database password.
2. SQL editor: `create extension if not exists vector;`
3. Get the **session-mode** pooler connection string: Project settings, Database, Connection string, Session pooler. It looks like `postgresql://postgres.<ref>:<password>@aws-0-<region>.pooler.supabase.com:5432/postgres`. This is `DATABASE_URL` for every service.
4. Link and push migrations from your machine:

```
supabase link --project-ref <ref>
supabase db push
```

`db push` applies the 21 forward-only migrations. They create the `vector` schema objects, RLS on every table, and the `brain_app`, `brain_worker`, `brain_embedder` roles granted to `postgres`. Run this before any service starts.

## 3. Supabase Storage

Same project as the database. Supabase Storage is S3-compatible, so the existing worker code uses it with no change.

1. Storage: create a private bucket `company-brain-payloads`.
2. The S3 endpoint is `https://<project-ref>.supabase.co/storage/v1/s3` and the region is the project region, for example `eu-north-1`.

For credentials there are two options. Dashboard S3 access keys work, but they can only be created by hand. The scripted alternative used here is **session-token auth**, which needs no dashboard step:

```
AWS_ACCESS_KEY_ID     = <project ref>
AWS_SECRET_ACCESS_KEY = <legacy anon JWT>
AWS_SESSION_TOKEN     = <legacy service_role JWT>
```

The Go AWS SDK reads all three from the environment via its default credential chain, so no code change is needed. The code turns on path-style addressing automatically whenever `S3_ENDPOINT` is set, which Supabase requires.

Note: the Storage admin API rejects the new `sb_secret_...` keys ("Invalid Compact JWS"); use the legacy JWT keys for storage.

## 4. Railway

Create one project. Add four services from the GitHub repo (or `railway up` from your machine). For each, set Root Directory and Config File per the topology table, then set variables.

### Shared variables (all services)

```
DATABASE_URL = <session-mode pooler string>
```

### web

```
Root Directory: /
Config File: apps/web/railway.json
Networking: generate a public domain
SERVING_URL                    = https://<serving public domain>
INTERNAL_API_SECRET            = <the shared secret>
SUPABASE_URL       = https://<project-ref>.supabase.co
SUPABASE_PUBLISHABLE_KEY = <supabase publishable key, sb_publishable_...>
```

`SUPABASE_URL` and `SUPABASE_PUBLISHABLE_KEY` are read on the server at **runtime** and handed to the login form as props. They are deliberately not `NEXT_PUBLIC_*`: Next.js would inline those into the browser bundle at build time, which breaks in a container image built without them and locks one image to one environment. Keeping them runtime means the same image promotes across environments unchanged.

Auth is Supabase email magic link. Sign-in works only for emails that exist as a `people` row (see step 6). Two settings in the Supabase dashboard, Authentication, URL Configuration:

- Site URL: `https://<web public domain>`
- Redirect URLs: add `https://<web public domain>/api/auth/callback`

Also configure SMTP under Authentication, Emails. Supabase's built-in email sender is rate-limited and not meant for production. Point it at Resend, Postmark, or SES so sign-in links actually deliver.

### serving

```
Root Directory: /
Config File: services/serving/railway.json
Networking: generate a public domain
PORT                 = provided by Railway, do not set
ANTHROPIC_API_KEY    = <key>
INTERNAL_API_SECRET  = <the shared secret, same as web>
EMBEDDING_MODEL      = voyage-large-2
VOYAGE_API_KEY       = <key>
SCHEDULER_ENABLED    = 1
```

`CHAT_MODEL`, `DRIFT_TIER2_MODEL`, `DRIFT_TIER3_MODEL`, and the interval variables have sane defaults; override only if needed. Serving binds `::` so Railway's IPv6 proxy can reach it.

### embedding

```
Root Directory: services/embedding
Config File: railway.json
Networking: none (worker)
DATABASE_SET_ROLE    = brain_embedder
EMBEDDING_PROVIDER   = voyage
VOYAGE_API_KEY       = <key>
VOYAGE_MODEL         = voyage-large-2
MASTER_KEY           = <the shared master key>
S3_BUCKET            = company-brain-payloads
S3_ENDPOINT          = https://<project-ref>.storage.supabase.co/storage/v1/s3
S3_REGION            = <project region, e.g. us-east-1>
AWS_ACCESS_KEY_ID    = <supabase storage access key id>
AWS_SECRET_ACCESS_KEY = <supabase storage secret>
```

### ingestion

```
Root Directory: services/ingestion
Config File: railway.json
Networking: none until Slack is connected
DATABASE_SET_ROLE    = brain_worker
MASTER_KEY           = <the shared master key, same as embedding>
S3_BUCKET            = company-brain-payloads
S3_ENDPOINT          = https://<project-ref>.storage.supabase.co/storage/v1/s3
S3_REGION            = <project region, e.g. us-east-1>
AWS_ACCESS_KEY_ID    = <supabase storage access key id>
AWS_SECRET_ACCESS_KEY = <supabase storage secret>
```

To receive Slack events later: generate a public domain for ingestion, set `WEBHOOK_ADDR = :$PORT` and `SLACK_SIGNING_SECRET`, and point the Slack app's request URL at `https://<ingestion domain>/webhooks/slack`.

## 5. Deploy order

1. `supabase db push` (migrations first, always).
2. serving.
3. web (needs the serving domain).
4. embedding.
5. ingestion.

## 6. Seed the first tenant

Sign-in only works for an email that already exists as a `people` row. With `psql` against the pooler, insert one tenant and one admin person whose `email` is the address you will sign in with. Do this as a normal insert. Do not disable RLS.

```sql
insert into tenants (id, name, tier)
values (gen_random_uuid(), 'Your Company', 'growth')
returning id;

insert into people (tenant_id, email, display_name, role)
values ('<tenant id from above>', 'you@yourcompany.com', 'Your Name', 'admin');
```

## 7. Smoke test

```
curl -sS https://<web domain>/            # landing page renders
curl -sS https://<serving domain>/ -i     # reachable, rejects without the secret
```

Then open the web app, go to `/login`, enter the admin email you seeded, click the magic link from your inbox, and confirm it lands you in the dashboard. From there ask the chat a question and confirm you get a receipted answer citing canon entry ids.

## 8. Redeploy from dev

Once each service is linked once, redeploy from your machine with `bin/deploy`. It runs `railway up` per service against the currently linked project. Run migrations yourself with `supabase db push` before deploying if a migration changed.
