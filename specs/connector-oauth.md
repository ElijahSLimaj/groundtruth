# Connector credential acquisition — current (2026) reference

Research-backed spec for how Groundtruth connectors acquire and store credentials. Drives `services/serving/src/connectors` and the web OAuth routes. Secrets are sealed with the AES-256-GCM envelope in `secret-crypto.ts` / `store.OpenConnectorSecret` (proven Go↔TS) and stored in `connectors.config.secret`.

## Baseline: OAuth 2.1 (write this, not 2.0)

Any new authorization-code flow in 2026 is OAuth 2.1, which folds in PKCE + the Security BCP. Non-negotiable for every OAuth provider below:

- **PKCE on every flow**, including our confidential server-side client. Generate `code_verifier` (43–128 chars, high-entropy), send `code_challenge = BASE64URL(SHA256(verifier))` + `code_challenge_method=S256` on authorize; send the `code_verifier` on token exchange.
- **`state`**: cryptographically-random, bound to the session, verified byte-for-byte on callback (CSRF defence).
- **Exact-match redirect URIs** — byte-for-byte, no wildcards/suffixes. One registered redirect per provider: `https://app.<domain>/api/connectors/<source>/callback`.
- **Refresh-token rotation** where the provider issues a new refresh token on each use — persist the new one; treat reuse of a retired token as a compromise signal.
- Access tokens are short-lived; refresh tokens are the durable secret → encrypted at rest (done).

**PKCE + state store.** Because serving runs multiple replicas, keep the per-flow `{state → code_verifier, tenant_id, person_id, source, created_at}` in a short-lived `oauth_flows` table (TTL ~10 min), not in memory. Idempotent, replica-safe, and lets the callback run on any instance.

## Two credential families

The "OAuth framework" must actually be a **credential-provider abstraction**, because not every source uses OAuth:

1. **OAuth 2.1 authorization-code + PKCE** — Slack, Google, Microsoft, Notion, HubSpot, Linear, Salesforce. Consent redirect → code → token exchange → sealed store.
2. **API-key / token paste** — Fathom, Odoo. No consent redirect; the admin pastes a key (or we register a webhook). Same sealed storage, different acquisition UI. Do **not** force these through an OAuth shape.

## Per-provider matrix

| Source | Family | Authorize | Token | Exchange auth | Refresh | Quirks |
|---|---|---|---|---|---|---|
| **slack** | OAuth | `slack.com/oauth/v2/authorize` | `slack.com/api/oauth.v2.access` | client id/secret in POST body | Opt-in **token rotation** (~12h access + refresh via `grant_type=refresh_token`); default is a non-expiring bot token | bot token `xoxb-` is nested at `access_token`; `scope=` for bot scopes, `user_scope=` for user; code valid 10 min. Opt into rotation for production. |
| **gmail / gdrive** | OAuth | `accounts.google.com/o/oauth2/v2/auth` | `oauth2.googleapis.com/token` | client id/secret + PKCE | `refresh_token` only when `access_type=offline`; add `prompt=consent` to guarantee it; refresh dies after 6 months unused | one Google consent can carry both Gmail + Drive scopes (incremental auth); readonly scopes are Google-restricted → security assessment for public use |
| **outlook / teams** | OAuth | `login.microsoftonline.com/{tenant}/oauth2/v2.0/authorize` | `.../oauth2/v2.0/token` | client id/secret + PKCE, scope includes `offline_access` | `refresh_token`; confidential-client refresh ~90 days | **Teams** channel messages need `ChannelMessage.Read.All` (delegated) or app perms + **admin consent**; org-wide Teams read is a **protected API** requiring Microsoft onboarding — real timeline gate |
| **notion** | OAuth | `api.notion.com/v1/oauth/authorize` | `api.notion.com/v1/oauth/token` | **HTTP Basic** (`client_id:client_secret`) | **none** — Notion tokens don't expire | user selects the pages/DBs to grant on the consent screen; `owner=user`; bearer afterward |
| **hubspot** | OAuth | `app.hubspot.com/oauth/authorize` | `api.hubapi.com/oauth/v1/token` | client id/secret in body | `refresh_token` (long-lived), access short | token response returns `scopes`, `hub_id`, `expires_in` (extra fields since Aug 2025); store `hub_id` as clear metadata |
| **linear** | OAuth | `linear.app/oauth/authorize` | `api.linear.app/oauth/token` | client id/secret | access **24h** + `refresh_token` | optional `actor=app` (actions attributed to the app, not the user) — but then cannot request `admin` scope |
| **salesforce** | OAuth | `login.salesforce.com/services/oauth2/authorize` | `.../services/oauth2/token` | client id/secret + PKCE (can be **required** on the connected app) | `refresh_token` | token response returns per-org **`instance_url`** — store as clear metadata; all later API calls target that host |
| **fathom** | API-key + webhook | — | — | `X-Api-Key` header (user-level key) | n/a | register webhook `POST /webhooks`, verify HMAC `webhook-signature` on delivery; pull `GET /external/v1/meetings?include_transcript=true` with `next_cursor`; transcripts lag a few minutes (async) |
| **odoo** | API-key | — | — | API key as bearer / `X-Api-Key` | n/a | per-instance base URL (self-host or SaaS). Odoo 19 **JSON-2 API** (bearer) is the target; XML-RPC/JSON-RPC removed in Odoo 20 (fall 2026) — build against JSON-2/REST bearer |

## Config shape (`connectors.config`)

```jsonc
{
  "secret": "<base64 AES-256-GCM of {access_token, refresh_token}>",  // sealed, AAD = connector:<id>
  "expires_at": "2026-08-02T18:00:00Z",   // clear, so the refresh job doesn't decrypt to check
  "scope": "channels:history ...",         // clear
  "account": "team T123 / hub 42 / instance https://acme.my.salesforce.com",  // clear metadata
  "source_scope": { ... }                  // clear, provider-specific (workspace, hub_id, instance_url)
}
```

## Refresh (per provider)

A serving scheduler job (advisory-locked, `withAdvisoryLock`) selects connectors whose clear `expires_at` is near, decrypts the refresh token, calls the provider token endpoint, **re-seals** the new secret (persisting a rotated refresh token if returned), updates `expires_at`. No-op for Notion/Slack-default/Fathom/Odoo (non-expiring or API-key).

## Verification discipline

Test each token exchange + refresh against the provider's **documented token-response JSON shape** (real payloads via httptest), the same discipline that caught the Outlook delta-endpoint bug — never fake-only. Provider app registrations (client id/secret) are the last-mile live test and gate public self-serve (Google restricted-scope assessment, Microsoft Teams protected-API onboarding, Slack directory review).

## Sources
OAuth 2.1 / PKCE BCP ([askmeidentity](https://askmeidentity.com/resources/standards/oauth-2-1-explained/), [authgear](https://www.authgear.com/post/oauth2-security-best-practices-pkce-state/)) ·
[Slack installing-with-oauth](https://docs.slack.dev/authentication/installing-with-oauth/) ·
[Google web-server OAuth](https://developers.google.com/identity/protocols/oauth2/web-server) ·
[Microsoft Entra auth-code flow](https://learn.microsoft.com/en-us/entra/identity-platform/v2-oauth2-auth-code-flow) · [Teams ChannelMessage.Read.All](https://graphpermissions.merill.net/permission/ChannelMessage.Read.All) ·
[Notion authentication](https://developers.notion.com/reference/authentication) ·
[HubSpot OAuth token changelog](https://developers.hubspot.com/changelog/additional-details-returned-when-generating-oauth-access-tokens) ·
[Linear OAuth](https://linear.app/developers/oauth-2-0-authentication) · [Linear actor auth](https://linear.app/developers/oauth-actor-authorization) ·
[Salesforce web-server + PKCE](https://help.salesforce.com/apex/HTViewHelpDoc?id=sf.remoteaccess_oauth_web_server_flow.htm) ·
[Fathom public API](https://help.fathom.video/en/articles/8368641) ·
[Odoo external API](https://www.odoo.com/documentation/19.0/developer/reference/external_api.html)
