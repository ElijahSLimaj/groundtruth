# Company Brain
## Full Specification

**Version** 1.1
**Date** July 2026
**Status** Complete, ready for build

This is the single canonical specification for Company Brain. It contains three parts.

**Part I, Product.** What the product is, who it is for, the business case, pricing, and non-goals.
**Part II, System.** Architecture, data model, canon governance, serving contract, scale posture, and reference flows.
**Part III, Build.** Every module at implementation depth, ingestion through infrastructure, with the cross-module build order.

---

-e 

# PART I, PRODUCT SPECIFICATION


**One-liner.** You cannot deploy AI into a company that is not machine-readable. Company Brain makes your company machine-readable, with human-governed truth.

---

## 1. What the Product Does

Company Brain is the governed knowledge layer for companies deploying AI. It captures everything a company produces, structures it into verified, owned, cited truth, and serves it to humans and AI agents with full provenance and permissions.

Every company runs on two kinds of knowledge. The deliberate kind, such as strategy, pricing, policies, and structure. And the ambient kind, the thousands of messages, documents, meetings, and decisions produced every day. Today the first kind rots in wikis nobody maintains, and the second kind is unsearchable noise. AI agents deployed into this environment hallucinate company facts, and nobody trusts them.

Company Brain solves this with a two-layer architecture and a pipe between them. A small, curated, human-approved canon holds the company's ground truth. A raw, high-volume stream holds everything else. A drift engine continuously watches the stream for signals that the canon is out of date, drafts the correction, and routes it to a human owner for approval. The canon stays small and true. The stream stays raw and searchable. Agents and employees query both through one interface and always know which is which.

The promise is process integrity, not AI magic. Every fact served traces to source events, a named human approver, and a verification date. The product does not warrant that the content is true. It warrants that a named person at the company approved it, on a known date, from known sources, and that staleness and conflicts are surfaced rather than hidden.

## 2. Who It Is For

**Primary buyer.** Founder or COO at companies of roughly 30 to 300 employees. Large enough to suffer knowledge chaos, too small for enterprise search budgets, ambitious enough to be deploying AI agents now.

**Primary users.**

The **domain owner** (head of sales, finance lead, ops manager) who approves drift proposals for their slice of the canon in minutes per week.

The **employee** who asks questions and gets cited, confidence-labeled answers instead of interrupting a senior colleague.

The **AI agent**, a first-class citizen of the system, querying the brain through a standard interface with its own identity, permissions, and rate limits.

## 3. The Business Problems It Solves

**Onboarding cost.** New hires burn two to three months reaching productivity because knowledge lives in heads and scattered tools. The canon literally explains the company, compressing ramp time.

**Decision amnesia.** Decisions get made in Slack, never written down, and re-litigated quarterly. The drift engine detects decisions in the stream and proposes them into the canon, making them durable.

**AI readiness.** This is the wedge. Every agent deployment stalls on the same wall. The agent does not know the company. Without a governed knowledge layer, agents hallucinate policy and pricing, and trust collapses. With Company Brain, every AI tool the company adopts gets smarter on day one.

## 4. Core Concepts

**The Canon.** The company's ground truth. A deliberately small, structured set of entries covering strategy, org structure, product, pricing, sales, finance, operations, and policies. Every entry has a single named owner, a plain-language statement, links to the source events it came from, an approval record, a verified-at date, a decay policy, a visibility scope, and typed relations to other entries (supersedes, conflicts with, depends on). The canon is versioned. Every change is attributable and reversible. The canon is hard-capped by company size, in the hundreds of entries, never tens of thousands. Small and true beats big and comprehensive.

**The Stream.** Everything else. Messages, emails, documents, meeting transcripts, tickets, commits, flowing in continuously from connected tools. High volume, low trust, explicitly labeled as such. The stream is searchable but never treated as ground truth. Its job is to be the raw signal from which the drift engine works.

**Drift.** The gap between what the canon says and what the company is actually doing and saying. Drift is the disease every knowledge base dies of. Detecting and closing it is the product.

**Provenance.** Every canon entry traces back to the real events that produced it. The system never invents facts. It only proposes facts extracted from what the company actually said and did.

## 5. Modules

### 5.1 Ingestion

Connects to the tools where company knowledge is actually produced. Chat (Slack), email, document stores (Drive, Notion), code and tickets (GitHub, Linear, Jira), meeting transcripts, and the HR system for org structure.

Every incoming event is normalized into a common format carrying its source, author, timestamp, and, critically, the access permissions of its origin. A message from a private channel carries that channel's visibility forever, through every transformation downstream. Permissions are inherited at the moment of capture, not reconstructed later.

Ingestion is continuous. The brain is always current with the tools it watches.

### 5.2 The Stream

The normalized, searchable record of everything ingested. Retention policies are configurable per source. Queries against the stream always return results labeled as unverified signal, clearly distinguished from canon answers. Access control from the source systems is enforced at query time on every request, so a user or agent can never retrieve something they could not have seen in the original tool.

### 5.3 The Canon

The structured home of company truth, organized into domains, each with an accountable owner. Owners are the human governance layer. Nothing enters, changes, or leaves the canon without an owner's explicit approval.

The canon actively resists bloat. Merge suggestions when entries overlap. Decay flags when entries pass their verification window without review. Forced ownership so no entry is orphaned. A per-domain health score, visible to leadership, that makes neglect impossible to hide. Social pressure is part of the accuracy system.

### 5.4 The Drift Engine

The heart of the product. It continuously compares the stream against the canon and produces three kinds of output.

**Contradiction alerts.** Recent activity disagrees with a canon entry. Sales quoted a new price three times this week, but the canon says otherwise.

**Gap proposals.** A topic recurs in the stream with no canon coverage. The team keeps discussing a process that was never written down.

**Decay flags.** A canon entry has gone unreferenced or unverified past its window. It may still be true, but the system no longer vouches for its freshness.

Each output arrives as a drafted canon change, written by the system from the source events, routed to the responsible owner. The owner approves, edits, or rejects in one action, from the web app or directly inside Slack. Rejections feed back into the engine, tuning its thresholds to each company.

The engine is tuned for precision over recall. Five high-confidence alerts a week that are always right beat fifty maybes. Owner attention is the scarcest resource in the system, and the engine treats it that way.

### 5.5 Serving

One interface for humans and agents alike.

**For agents.** A standard machine interface (MCP-native, plus a conventional API) through which any agent framework, on any vendor's models, queries the brain. Answers come back canon-first with citations, confidence levels, and freshness. When the canon has no coverage, the answer says so explicitly. An agent responding "I do not have governed knowledge on this" is a correct answer, and often the most important one. Agents are principals in their own right, with scoped credentials, their own permission sets, and rate limits.

**For humans.** A web app with a canon browser, the approval queue, a drift dashboard, connector management, and the full audit log. A Slack app brings approvals into the owner's existing workflow so governance never requires leaving the tools they already live in.

**Answer contract.** Every answer, human or agent, carries three things. What the answer is, where it came from (one click to the receipt), and how much to trust it (canon-verified, canon-stale, stream-only, or no coverage). Conflicts are never silently resolved. If the canon says one thing and the stream suggests another, both are served, labeled, with the conflict flagged for the owner.

### 5.6 Cold Start

On first connection, the system processes the last ninety days of company history and drafts an initial canon. Inferred org chart, detected decisions, pricing signals, recurring processes. It is presented as a review queue, not a finished wiki. The founder spends an afternoon approving and correcting instead of months writing from scratch.

Nobody hand-authors a knowledge base in 2026. Cold start is the onboarding, the demo, and the sales pitch in a single feature. Time from connecting the first tools to a reviewed, living canon is measured in hours.

### 5.7 Trust and Governance

Full audit trail on every entry and every answer served. Per-entry access control mirrored from source systems with manual overrides. Four roles, admin, domain owner, member, and agent. Single sign-on and enterprise identity support from day one, because the buyer's security review is part of the sales cycle, not a later phase.

The accuracy contract, stated plainly. The system guarantees provenance, approval, freshness enforcement, conflict surfacing, and calibrated answers. The customer owns the truth itself. Owners must actually review, and knowledge that never touches a connected channel cannot be captured. The brain knows what the company expressed, not what it secretly thinks. The product's job is to make governance take minutes per week and to make neglect visible.

## 6. How It Works, End to End

1. The company connects its tools. Cold start drafts the initial canon from ninety days of history. Owners review and approve. Within a day, the company has a living, governed brain.
2. From then on, ingestion runs continuously. Every message, document, meeting, and commit flows into the stream with its permissions intact.
3. The drift engine watches. When the stream contradicts the canon, when a gap appears, or when an entry goes stale, the responsible owner gets a drafted change and approves or rejects it in one action.
4. Employees ask questions and get cited, confidence-labeled answers. Agents query the same brain through the machine interface and act on governed truth instead of hallucinated policy.
5. Every answer served, every change approved, every conflict surfaced lands in the audit trail. The COO watches canon health per domain. Trust compounds.

## 7. Pricing

The model matches what the product is. Infrastructure that agents depend on, not a wiki people browse. No per-seat pricing, ever. Per-seat frames the product as a collaboration tool, invites seat haggling, and punishes exactly the behavior the product wants to encourage.

**Platform fee, tiered by company size and connectors.**

**Core**, around $500 per month. Up to 50 employees, four connectors. The full product, no crippled edition.

**Growth**, around $1,500 per month. Up to 300 employees, all connectors, single sign-on.

**Scale**, from $4,000 per month. Custom retention, audit exports, dedicated tenancy, compliance reporting.

**Metered agent usage on top.** Human access to the brain is free and unlimited. Human engagement keeps the canon alive and is never taxed. Agent queries through the machine interface are metered, with a generous included volume per tier and usage pricing beyond it. Revenue scales with the customer's agent adoption, which is precisely the trend the product is built on. Early on, the platform fee carries the account. As agents proliferate, usage becomes the majority of revenue without a single price increase.

**No free tier.** Cold start is expensive to run and the product deserves a serious buyer. Instead, a paid pilot. Thirty days, full product, cold start included, money back if the drafted canon is not useful.

**Why churn approaches zero.** Once several of the customer's agents cite the canon as ground truth, removing the product means every agent goes dumb simultaneously. The canon itself is accumulated, approved company truth that nobody rebuilds to save $1,500 a month. Switching cost compounds monthly.

## 8. What Makes It the Best Tool in the Category

**The drift pipe actually works.** Everyone else ships search over a pile of embeddings. This product ships the approval loop that keeps company truth true. Precision-tuned, owner-routed, feedback-trained. Boring, unsexy, and the entire game.

**Permissions are structural.** Inherited at capture, enforced at query, never bolted on. One leak of an executive discussion to an intern kills the category for a vendor. The data model makes that leak impossible by construction.

**Value in under an hour.** Cold start turns the empty-wiki problem into an afternoon of approvals.

**The standard interface for agents.** Any agent, any framework, any model vendor plugs in and gets governed answers. Distribution flows through other people's agents, the way payments flow through Stripe.

**Provable trust.** Every answer has a receipt. That is what lets a COO defend the tool internally and what carries an enterprise security review.

**Ruthless canon discipline.** The product actively fights bloat. Small and true beats big and comprehensive, and it is the reason this knowledge base survives where twenty years of wikis have died.

## 9. Success Metrics

**North star.** Agent queries per tenant per week. It predicts both revenue and retention. If it grows, everything else follows.

**Product health.** Drift proposal acceptance rate (precision proxy, target above 70 percent). Median owner time in the approval queue per week (target under 15 minutes). Canon health score trend per domain. Time from connection to reviewed canon (target under one day).

**Commercial health.** Pilot to paid conversion (target above 60 percent). Usage revenue as a share of total, rising quarter over quarter. Net revenue retention above 120 percent, driven by agent adoption, not price increases.

## 10. Explicit Non-Goals

Not a horizontal enterprise search engine. Glean owns that fight.

Not a chat-with-your-docs interface. That category is a feature, not a company.

Not an agent framework. The product feeds every agent, competes with none.

Not a warranty on facts. The product warrants process integrity, provenance, approval, freshness, and conflict surfacing. The truth itself belongs to the customer.
-e 

---

# PART II, SYSTEM SPECIFICATION


## 1. System Overview

Company Brain captures everything a company produces, structures it into verified, owned, cited truth, and serves it to humans and AI agents with full provenance and permission enforcement.

The architecture rests on two layers and the pipe between them.

**The canon.** Small, curated, human-approved ground truth. Strategy, org structure, product, pricing, sales, finance, ops, policies. Hundreds of entries, never tens of thousands. Every entry has one named owner, provenance to source events, an approval record, a verification date, and a decay policy.

**The stream.** The raw, high-volume record of everything ingested. Messages, emails, documents, transcripts, tickets, commits. Searchable by meaning, explicitly labeled low-trust, never treated as ground truth.

**The drift engine.** The pipe. Continuously compares stream against canon, detects contradictions, gaps, and decay, drafts the correction, and routes it to a human owner for approval. This is the core product. Everything else exists to feed it or serve its output.

**The accuracy contract.** The system warrants process integrity, not factual truth. Every fact served traces to source events, a named approver, and a verification date. Staleness and conflicts are surfaced, never hidden. Content truth belongs to the customer. Process truth belongs to the system.

## 2. Architecture

Five services around one database.

**Ingestion service (Go).** Connector runtime. Webhook-first, polling fallback. Normalizes every source item into an event carrying source, author, timestamp, thread key, and the ACL of its origin. Horizontally scalable worker pool feeding a queue. Ingestion is the write-amplification hotspot and scales independently of everything else.

**Embedding pipeline (Go workers).** Consumes new events from the queue, chunks, embeds, writes event_chunks with denormalized ACLs. Rebuildable at any time by replaying the event log, so nothing precious lives in the vectors.

**Drift engine (Node/TypeScript, model orchestration).** Three-tier cascade detailed in section 6. Produces drift proposals routed to owners.

**Serving API (NestJS or Go).** MCP server as the primary interface, REST alongside. Canon-first retrieval with citations, confidence, freshness labels, and query-time ACL enforcement. Agents are first-class principals.

**Web app (Next.js).** Canon browser, approval queue, drift dashboard, connector management, canon health scores, audit log. Slack app mirrors the approval queue so owners never leave their workflow.

**Database.** Single Postgres cluster with pgvector. Multi-tenant via row-level security on every table. Per-tenant encryption keys for payloads in object storage. Raw bodies and attachments live in object storage, the hot database stays lean.

Model usage runs Anthropic models for drift comparison and drafting, with a cheap-model triage tier in front. Embedding model is swappable by design, re-embed from the event log and drop the old index.

## 3. Connectors

Launch set, all in scope from day one.

**Communication.** Slack (channels and threads, DMs excluded by default), Gmail and Outlook, meeting transcripts via Fathom, Zoom, Meet, or an owned bot. Transcripts are the highest signal per word of any source.

**Documents.** Google Drive, OneDrive, Notion, Confluence. Existing wikis are cold-start fuel.

**Work systems.** GitHub or GitLab, Linear or Jira, CRM (HubSpot, Salesforce). The CRM is where canon-versus-reality drift shows up first.

**Structure.** HRIS (Personio, BambooHR, Deel) for org chart, roles, start dates. Feeds owner routing and cold start.

**Finance (high-sensitivity tier).** Stripe, QuickBooks. Read-only, heavily scoped.

Connector priority rule. A connector earns its place by drift signal per event, not volume. CRM produces few events and nearly every one matters. Slack produces millions and most are noise. Both are required because contradictions live in the noisy ones.

### 3.1 Gmail connector, detailed

Org-level access via Google Workspace admin grant, no per-employee OAuth chase. Push notifications per mailbox, no constant polling. Cold start pulls 90 days per mailbox. Threads are stitched so a decision spread across 14 replies reads as one conversation.

Permission model, non-negotiable.

1. An email's visibility is its recipient list. Only thread participants can retrieve it from the stream. No admin override on raw content.
2. Canon proposals carry the extracted fact and a minimal excerpt, never the mailbox.
3. Ingestion-time exclusion of HR, legal, medical, and payroll patterns. Filtered before storage, not hidden at query time. Toxic content is never stored.
4. Per-person opt-out and per-label exclusions.

Default deployment posture is org-wide with aggressive exclusions rather than per-mailbox opt-in, because partial email coverage guts drift detection. The pilot security conversation is won with recipient-list visibility plus ingestion-time filtering plus the audit log.

## 4. Data Model

Postgres. SQL was chosen over NoSQL because the provenance chain is a chain of foreign keys the database itself guarantees, the approval flow needs ACID transactions, tenancy needs row-level security, and pgvector keeps semantic search in the same engine with no sync seams. The category-killing risk is a permission leak, and leaks live in seams between systems.

Three shapes of data, deliberately separated.

**The event log.** Append-only, immutable. Insert forever, never update. Enables reprocessing when extraction models improve and provides provenance forever.

**The vector index.** An access path over the event log, rebuildable, carrying denormalized ACLs so similarity queries filter permissions inside the query.

**The canon.** Fully relational, versioned like Git. Current state is a pointer into an immutable version history, giving free audit, free rollback, and answers to "what did the canon say on March 3rd."

### 4.1 Schema

```sql
create table tenants (
  id uuid primary key default gen_random_uuid(),
  name text not null,
  tier text not null,
  created_at timestamptz not null default now()
);

create table people (
  id uuid primary key default gen_random_uuid(),
  tenant_id uuid not null references tenants(id),
  email text not null,
  display_name text not null,
  role text not null check (role in ('admin', 'owner', 'member', 'agent')),
  unique (tenant_id, email)
);

create table connectors (
  id uuid primary key default gen_random_uuid(),
  tenant_id uuid not null references tenants(id),
  source_type text not null,
  status text not null,
  config jsonb not null,
  created_at timestamptz not null default now()
);

create table events (
  id uuid primary key default gen_random_uuid(),
  tenant_id uuid not null references tenants(id),
  connector_id uuid not null references connectors(id),
  source_type text not null,
  external_id text not null,
  author_id uuid references people(id),
  thread_key text,
  occurred_at timestamptz not null,
  ingested_at timestamptz not null default now(),
  acl jsonb not null,
  payload_ref text not null,
  unique (tenant_id, connector_id, external_id)
) partition by range (occurred_at);

create table event_chunks (
  id uuid primary key default gen_random_uuid(),
  tenant_id uuid not null references tenants(id),
  event_id uuid not null references events(id),
  chunk_index int not null,
  content text not null,
  embedding vector(1536) not null,
  acl jsonb not null
) partition by range (created_at);

create table canon_entries (
  id uuid primary key default gen_random_uuid(),
  tenant_id uuid not null references tenants(id),
  domain text not null,
  tier text not null check (tier in ('bedrock', 'operational')),
  owner_id uuid not null references people(id),
  current_version_id uuid,
  status text not null check (status in ('active', 'decayed', 'archived')),
  visibility jsonb not null,
  verify_interval interval not null,
  created_at timestamptz not null default now()
);

create table canon_versions (
  id uuid primary key default gen_random_uuid(),
  entry_id uuid not null references canon_entries(id),
  version_number int not null,
  statement text not null,
  created_by uuid not null references people(id),
  created_at timestamptz not null default now(),
  unique (entry_id, version_number)
);

create table canon_provenance (
  version_id uuid not null references canon_versions(id),
  event_id uuid not null references events(id),
  primary key (version_id, event_id)
);

create table canon_relations (
  from_entry uuid not null references canon_entries(id),
  to_entry uuid not null references canon_entries(id),
  relation text not null check (relation in ('supersedes', 'conflicts_with', 'depends_on')),
  primary key (from_entry, to_entry, relation)
);

create table approval_policies (
  id uuid primary key default gen_random_uuid(),
  tenant_id uuid not null references tenants(id),
  tier text not null,
  domain text,
  required_role text not null,
  required_approver_count int not null default 1
);

create table approvals (
  id uuid primary key default gen_random_uuid(),
  version_id uuid not null references canon_versions(id),
  approver_id uuid not null references people(id),
  decision text not null check (decision in ('approved', 'rejected')),
  decided_at timestamptz not null default now(),
  note text
);

create table drift_proposals (
  id uuid primary key default gen_random_uuid(),
  tenant_id uuid not null references tenants(id),
  entry_id uuid references canon_entries(id),
  kind text not null check (kind in ('contradiction', 'gap', 'decay')),
  drafted_statement text not null,
  confidence numeric not null,
  routed_to uuid not null references people(id),
  status text not null default 'pending',
  resolution text,
  created_at timestamptz not null default now()
);

create table audit_log (
  id bigint generated always as identity primary key,
  tenant_id uuid not null references tenants(id),
  actor_id uuid references people(id),
  action text not null,
  subject_type text not null,
  subject_id uuid,
  occurred_at timestamptz not null default now()
) partition by range (occurred_at);
```

### 4.2 Row-level security

Enabled on every table. The database refuses cross-tenant reads even when application code is buggy. Defense in depth at the lowest layer.

```sql
alter table events enable row level security;

create policy tenant_isolation on events
  using (tenant_id = current_setting('app.tenant_id')::uuid);
```

Same policy pattern applied to all tables.

### 4.3 Design decisions

- events is append-only. Canon history lives in canon_versions where every edit is a new immutable row and current_version_id is a moving pointer.
- entry_id on drift_proposals is nullable because gap proposals reference no existing entry.
- ACLs are jsonb because every source system expresses permissions differently. They are denormalized onto event_chunks so vector queries filter permissions without a join.
- Approval rules are data, not code. approval_policies drives the approval flow, so "bedrock requires admin" or "pricing requires two approvers" is configuration, never a hardcoded branch.
- events, event_chunks, and audit_log are partitioned by month from day one. Retrofitting partitioning onto a live table with foreign keys is avoidable pain.
- Raw payloads live in object storage behind payload_ref with per-tenant encryption keys.

## 5. Canon Governance

### 5.1 Two tiers, one store

**Bedrock.** ICP, GTM strategy, brand and design system, mission, positioning. Changes a few times a year. Long verify intervals, around 6 months. Stricter approval policy, admin role or multiple approvers. A contradiction alert against bedrock is a strategic signal and escalates rather than routing as a routine correction.

**Operational.** Pricing, discount policy, processes, org facts. Changes monthly. 30 to 60 day verify intervals, standard owner approval.

Both tiers live in the same tables. Separating bedrock into its own store was considered and rejected. Provenance requires foreign keys into the event log, the serving layer promises one query interface with uniform ACL filtering, and depends_on edges between operational and bedrock entries need referential integrity. Blast-radius concerns are answered by point-in-time recovery plus the immutable version history. One canon, tiered governance.

### 5.2 Bloat resistance

Hard cap on entry count scaled to company size. Merge suggestions when entries overlap. Decay flags when verification windows lapse. Forced ownership, no orphaned entries. Per-domain canon health score surfaced to leadership, making neglect visible. Social pressure is part of the accuracy system.

## 6. Drift Engine

The core product. Three-tier cascade, tuned for precision over recall. Five high-confidence alerts a week that are always right beat fifty maybes, because owner attention is the scarcest resource in the system.

**Tier 1, heuristics and embeddings.** Filters roughly 95 percent of events as irrelevant to canon. Similarity against canon entry embeddings, source and author weighting, near-free.

**Tier 2, cheap model classification.** Haiku-class model classifies survivors for potential contradiction, gap, or decay relevance.

**Tier 3, frontier model comparison and drafting.** Deep comparison against the candidate canon entry, drafts the proposed change with the minimal supporting excerpts and full event references.

The cascade is the unit economics. Running everything through a frontier model costs 20x more and kills margin.

**Outputs.**

- Contradiction alert. Stream disagrees with canon. Sales quoted a new price three times this week, canon says otherwise.
- Gap proposal. Recurring stream topic with no canon coverage.
- Decay flag. Entry unreferenced or past its verification window. Its confidence is downgraded in served answers until re-verified.

**Routing and feedback.** Every output arrives as a drafted change routed to the entry owner, actionable in one step from web or Slack. Approve, edit, reject. Approval executes as a single ACID transaction, new version created, approval recorded, proposal resolved, all or nothing. Rejections feed per-tenant threshold tuning. The drift engine gets smarter about each company the longer it runs, which is also the data moat.

## 7. Serving Layer

### 7.1 Answer contract

Every answer, human or agent, carries three things. The answer. The receipt, one click from citation to canon entry to approver to source events. The trust level, one of canon-verified, canon-stale, stream-only, or no-coverage. Conflicts are never silently resolved. When canon and stream disagree, both are served, labeled, and the conflict is flagged to the owner.

"I do not have governed knowledge on this" is a correct answer and often the most important one.

### 7.2 MCP interface

Primary machine interface. Tools exposed.

- query. Canon-first retrieval with stream fallback, citations, confidence, freshness.
- get_entry. Fetch a canon entry with full provenance and version history.
- list_conflicts. Open contradictions visible to the caller.
- propose_update. Agents can submit drift proposals into the same owner-approval pipeline.

Agents are first-class principals with scoped credentials, their own ACLs, rate limits, and audit trails. Any agent framework on any model vendor plugs in. The product feeds every agent and ships none, staying the substrate rather than competing with agent vendors.

### 7.3 Retrieval mechanics

Canon retrieval is relational plus light synthesis. Stream retrieval is vector search over event_chunks with ACL filtering inside the query. Known sharp edge, ACL post-filtering degrades HNSW recall when the caller sees little. Mitigation is iterative search with over-fetch, and visibility-scoped index partitions for the common broad scopes. Solved in the query layer, not by adding a second database.

### 7.4 Human surfaces

Web app with canon browser, approval queue, drift dashboard, connector management, health scores, audit log. Slack app for approvals in-flow. Human access is unlimited and untaxed, because human engagement keeps the canon alive.

## 8. Cold Start

On connection, the system processes 90 days of history through the drift cascade and drafts an initial canon. Inferred org chart from HRIS and communication patterns, detected decisions, pricing signals, recurring processes. Presented as a review queue, never a finished wiki. The founder spends an afternoon approving instead of months writing. Time from first connector to reviewed living canon, under one day.

Cold start is onboarding, demo, and sales pitch in one feature. It costs 200 to 500 dollars of inference per company, which is why there is no free tier.

## 9. Security and Trust

- ACLs captured at ingestion, enforced at query time on every request, inherited through every transformation. Never reconstructed later, never filtered in the UI.
- Row-level security on all tables, tenancy enforced by the database itself.
- Per-tenant encryption keys on payload storage.
- Full audit trail on every entry change, every approval, every answer served.
- Four roles. Admin, domain owner, member, agent.
- SSO and SCIM from day one. SOC 2 posture from the start, the buyer's security review is part of the sales cycle.
- Ingestion-time exclusion of toxic categories. Never stored, not merely hidden.

## 10. Scale Posture

Calibration. 5 million events is small for this design. Partition pruning keeps queries on monthly slices. pgvector with HNSW handles tens of millions of chunks at millisecond latency. Canon stays in the hundreds of entries by policy at every company size, so the most valuable tables never face scale at all.

The real pressure point is write amplification in ingestion and drift inference spend, which is a queue and worker-pool problem living in the Go services, not a database problem.

Graduation threshold. At 100M+ chunks per tenant or sub-50ms vector latency demands, extract the vector index to a dedicated engine while Postgres remains the system of record. Clean extraction by design, vectors are a rebuildable access path. The event log, canon, and provenance never leave Postgres, their value is relational integrity.

## 11. Reference Flows

### 11.1 Sales call to quote

Fathom transcript webhooks into the stream. The customer's sales agent, subscribed to transcript events in its scope, assembles context. Stream queries for the prospect's full history, canon queries for current pricing, discount policy, and escalation paths, all with receipts. It drafts the offer at policy, flags exceptions to the human who owns them, and routes the draft to the account owner for approval. The sent offer flows back into the stream. A rep manually undercutting canon pricing three times becomes next week's contradiction alert.

### 11.2 Release to marketing campaign

Release announced in Slack arrives as a stream event. The marketing agent pulls positioning, tone of voice, personas, and banned claims from canon, and the feature's backstory from the stream. It drafts LinkedIn, Instagram, X, blog, and newsletter as one consistent package. Newsletter audience resolves from the canon's segmentation policy through the CRM connector at send time. The full package lands as one approval bundle with citations. Published assets flow back into the stream, and marketing copy contradicting product canon surfaces as drift.

### 11.3 Feedback to feature proposal

Support emails, call transcripts, NPS comments, and Slack complaints accumulate as ordinary events. The product agent clusters by theme, weights by frequency, recency, and account revenue. Before proposing, it checks canon for roadmap fit and prior decisions, and if the idea was already rejected it reports that the signal has doubled since rejection instead of re-proposing naively. The structured proposal carries evidence with receipts and confidence. Whatever the product owner decides becomes a canon entry, so the decision is never re-litigated from scratch.

The universal loop across all three. Trigger from the stream, context assembly canon-first with receipts, generation, human approval, outcome fed back into the stream and sometimes the canon.

## 12. Pricing

Infrastructure pricing, never per-seat. Per-seat frames a wiki, invites haggling, and punishes agent adoption.

- **Core**, around 500 dollars per month. Up to 50 employees, 4 connectors, full product.
- **Growth**, around 1,500 dollars per month. Up to 300 employees, all connectors, SSO.
- **Scale**, from 4,000 dollars per month. Custom retention, audit exports, dedicated tenancy, compliance reporting.

Metered agent queries on top with generous included volume per tier. Human access free and unlimited. Usage revenue scales with the customer's agent adoption and carries 90 percent plus gross margin.

No free tier. Paid pilot instead, 30 days, full product, cold start included, money back if the drafted canon is not useful.

Unit economics at Growth tier. Roughly 200 to 300 dollars all-in tenant cost against 1,500 dollars revenue, 80 to 85 percent gross margin, improving with agent adoption because metered queries are the cheapest thing served.

## 13. Customer Definition

Founder, COO, or Head of AI at 30 to 300 person companies actively deploying AI agents. Trigger moments, a failed agent pilot, a key departure taking undocumented knowledge, or visibly slow onboarding. Qualifying question for the pilot, deploying agents within 6 months, and if not, walk away, because without agent ambition the product is a better wiki and better wikis do not retain.

Secondary channel. Agent-builder agencies deploying into client companies, who need a governed knowledge layer on every engagement. One agency is a distribution channel, not a customer.

## 14. Success Metrics

**North star.** Agent queries per tenant per week. Predicts both revenue and retention.

**Product health.** Drift proposal acceptance rate above 70 percent. Median owner approval time under 15 minutes per week. Canon health trend per domain. Connection to reviewed canon under one day.

**Commercial health.** Pilot conversion above 60 percent. Usage share of revenue rising quarterly. Net revenue retention above 120 percent driven by agent adoption, not price increases.

## 15. Non-Goals

- Not horizontal enterprise search. Glean owns that fight.
- Not chat-with-your-docs. A feature, not a company.
- Not an agent framework. The product feeds every agent and competes with none.
- Not a warranty on facts. The warranty is process integrity, provenance, approval, freshness, and conflict surfacing. The truth itself belongs to the customer.
-e 

---

# PART III, BUILD SPECIFICATION


# Module 01, Ingestion and Event Log

## 1.1 Purpose

Turn everything a company produces across its tools into a single, normalized, permission-carrying, immutable event log. Every downstream capability is only as good as this layer.

Two invariants override everything else in this module.

**Invariant 1, no silent loss.** Every source item either becomes an event or lands in a dead-letter queue with a reason. There is no third outcome.

**Invariant 2, ACL at capture.** An event's permissions are resolved and attached at ingestion, from the source system's own permission model. Never reconstructed later, never inferred, never defaulted to visible.

## 1.2 Architecture

```
Source systems
   |  webhooks (primary) / polling (fallback) / backfill (cold start)
   v
Connector runtime (Go)
   |  raw item + source ACL context
   v
Normalizer (per source type)
   |  NormalizedEvent
   v
Ingestion queue (durable)
   |
   +--> Event writer --> Postgres events + object storage payload
   +--> Dead-letter queue (on normalization or write failure)
```

One Go service, three packages with interfaces between them, connector runtime, normalization, persistence. Connectors know their source API and nothing about Postgres. The writer knows Postgres and nothing about Slack. The normalizer is the only place both shapes meet.

## 1.3 The Connector Contract

```go
type Connector interface {
    SourceType() string
    Subscribe(ctx context.Context, cfg ConnectorConfig, sink RawItemSink) error
    Poll(ctx context.Context, cfg ConnectorConfig, cursor Cursor, sink RawItemSink) (Cursor, error)
    Backfill(ctx context.Context, cfg ConnectorConfig, window BackfillWindow, sink RawItemSink) error
    ResolveACL(ctx context.Context, cfg ConnectorConfig, item RawItem) (ACL, error)
    HealthCheck(ctx context.Context, cfg ConnectorConfig) ConnectorHealth
}
```

- Subscribe is the primary path. Poll is the fallback driven by a persisted cursor. Backfill serves cold start through the same sink. One pipeline, three entry points, no separate backfill code path to drift out of sync.
- ResolveACL is separate because ACL resolution often needs extra API calls and must be independently retryable. A raw item without a resolved ACL never proceeds.
- Connectors are stateless. Cursors, tokens, and webhook registrations live in connector config and a connector_state table, any worker can run any connector.

## 1.4 Normalized Event

```go
type NormalizedEvent struct {
    TenantID    uuid.UUID
    ConnectorID uuid.UUID
    SourceType  string
    ExternalID  string
    AuthorRef   AuthorRef
    ThreadKey   string
    OccurredAt  time.Time
    ACL         ACL
    Payload     Payload
}
```

- **ExternalID.** The source's own stable identifier. Uniqueness of (tenant, connector, external_id) is the idempotency backbone.
- **AuthorRef.** Source identity resolved to a people row when a match exists, stored raw otherwise. Resolution is eventually consistent, never blocks ingestion.
- **ThreadKey.** Deterministic stitching key per source, Slack thread_ts, Gmail thread id, meeting id for transcript segments.
- **OccurredAt.** When it happened in the source, never when ingested. The gap to ingested_at is the ingestion-lag metric.
- **Payload.** Body plus source-specific structure in object storage under a per-tenant key, referenced by payload_ref. The events table stays metadata-only.

## 1.5 ACL Model

```json
{
  "scope": "principals",
  "principals": ["person:uuid"],
  "source_scope": { "type": "slack_channel", "id": "C0123", "visibility": "private" }
}
```

scope is one of principals (explicit list), group (resolvable audience), or tenant (company-visible). source_scope preserves original context for audit and re-resolution.

Per-source mapping.

- **Slack.** Public channel maps to tenant. Private channel maps to principals from membership at ingestion. DMs excluded by default at the connector, never ingested.
- **Gmail.** Always principals, the recipient list. No broader scope exists for email, ever. Ingestion-time exclusion filters (HR, legal, medical, payroll patterns, user-defined labels) run before normalization, matches are counted but never stored.
- **Drive and Notion.** Sharing settings map to principals, tenant, or group with the folder as source_scope.
- **CRM, tickets, code.** Generally tenant by convention, per-connector overrides in config.

Membership drift policy. ACLs are point-in-time by default, matching source-system behavior. A per-tenant strict mode re-resolves group scopes on a schedule for compliance-heavy customers.

## 1.6 Idempotency and Delivery

At-least-once delivery plus idempotent writes equals exactly-once effect.

- Unique constraint on (tenant_id, connector_id, external_id) absorbs duplicates as counted no-ops.
- Payload writes are content-addressed, rewrites harmless.
- Queue consumers ack only after the database transaction commits.

Edits become new events with the same thread_key and an edit marker referencing the original external id. Source deletions become tombstone events, downstream layers exclude tombstoned originals from retrieval while the audit trail retains existence. Physical deletion happens only through tenant offboarding or a verified erasure request, a separate administrative flow with its own audit entry.

## 1.7 Failure Handling

- **Webhook loss.** Every webhook connector also runs a low-frequency reconciliation poll against the latest cursor. Webhooks are an optimization, polls are the guarantee.
- **Token expiry.** Scheduled HealthCheck, auth failure flips the connector to degraded, notifies the tenant admin, pauses that connector's consumption only, resumes from cursor on recovery.
- **Normalization failure.** DLQ with raw payload and reason. DLQ depth pages. Items replay after a normalizer fix.
- **Rate limits.** Per-connector token bucket honoring source limits. Backfill runs at lower priority than live traffic.
- **Partial backfill failure.** Windows chunked per day per source, chunk completion recorded, failed chunks retry independently.

## 1.8 Observability

Per tenant per connector. Ingestion lag p50 and p99, events written, duplicates dropped, DLQ depth, exclusion hits, ACL resolution failures (these page, invariant 2 means such items are stuck, not defaulted), connector status transitions. Every event carries a trace id from webhook receipt to commit.

## 1.9 Testing Requirements

- Contract suite every connector must pass, duplicate delivery, out-of-order delivery, ACL failure, cursor resume after crash, backfill overlapping live traffic.
- Golden-file tests per normalizer, raw payload in, NormalizedEvent out.
- A permission test per connector asserting a private item's ACL never widens through normalization.

## 1.10 Build Order

1. Event writer, queue, schema, idempotency, DLQ.
2. Connector contract, runtime, cursors, health checks.
3. Slack connector, richest ACL and highest volume, hardens the pipeline.
4. Gmail, hardest permission model plus exclusion filters.
5. Drive, transcripts, CRM, then the rest.
6. Reconciliation polling, backfill chunking.
7. Dashboards and alerts.

---

# Module 02, Embedding Pipeline and Stream Retrieval

## 2.1 Purpose

Make the unstructured stream searchable by meaning, with permissions enforced inside every query. Consumes the event log, produces event_chunks, serves stream retrieval to the drift engine and the serving layer. Everything here is a rebuildable access path, nothing precious lives in the vectors.

## 2.2 Chunking Strategy

Chunking is per source type because message-shaped, email-shaped, document-shaped, and transcript-shaped text retrieve differently.

- **Chat messages.** Individually too small to embed usefully. Group by thread_key into windows, a thread up to roughly 800 tokens becomes one chunk, longer threads split at reply boundaries with one-message overlap. Standalone channel messages group into channel-plus-hour windows.
- **Email.** One chunk per message with quoted history stripped, the thread is reconstructable via thread_key, re-embedding quoted text just duplicates vectors. Long emails split at paragraph boundaries, target 500 to 800 tokens, 80 token overlap.
- **Documents.** Split at heading boundaries first, then paragraphs, target 600 tokens, 100 token overlap. Heading path stored in chunk metadata so retrieval can show "Pricing doc, section Discounts."
- **Transcripts.** Split at speaker-turn boundaries into 60 to 120 second windows, speaker labels retained in content, meeting id as thread_key.
- **Tickets and CRM.** One chunk per item body plus one per substantial comment.

Every chunk stores content, token count, chunk_index, source metadata, and the denormalized ACL of its event.

## 2.3 Embedding Model Abstraction

```typescript
interface EmbeddingProvider {
  modelId(): string;
  dimensions(): number;
  embed(batch: string[]): Promise<number[][]>;
}
```

- event_chunks carries an embedding_model column. Retrieval always filters on the active model id.
- Model migration is replay. Spin up workers embedding from the event log under the new model id into new index partitions, cut retrieval over when backfill completes, drop old partitions. Zero downtime by design.
- Batching at the provider's optimal batch size, retry with backoff, dead-letter on repeated failure, chunk-level not event-level, so one poisoned chunk never blocks a document.

## 2.4 Index and Retrieval

- HNSW index via pgvector, built concurrently, maintenance_work_mem sized for build.
- **Hybrid retrieval.** Vector similarity plus Postgres full-text (BM25-style ranking) merged with reciprocal rank fusion. Names, SKUs, and exact phrases are where pure vector search embarrasses itself.
- **ACL enforcement inside the query.** Every retrieval carries the caller's principal set. The where clause filters chunks whose acl admits the caller, evaluated in-query, never post-hoc in application code.
- **The recall sharp edge.** ACL filtering after HNSW degrades recall for narrow-visibility callers. Mitigation, iterative over-fetch, request k times 4, filter, repeat with doubled ef_search until k results or a hard cap of three iterations. For the common broad scopes, tenant-visible chunks live in a visibility-scoped partial index so the majority of queries never pay the over-fetch cost.
- **Freshness weighting.** Retrieval score blends similarity with a recency decay term, half-life configurable per source type, transcripts decay fast, documents slowly.
- Tombstoned events' chunks are excluded by a status flag checked in every retrieval.

## 2.5 Observability

Embedding lag from event commit to chunk indexed, p50 and p99. Retrieval latency by caller scope breadth. Over-fetch iteration histogram, if iteration three is common, the partition scheme needs revisiting. Recall spot-checks against exhaustive scan on a sampled query set, weekly, per tenant size class.

---

# Module 03, Canon and Approval Flow

## 3.1 Purpose

The structured home of company truth and the machinery that keeps every change owned, approved, versioned, and provable. The most valuable and most boring module. Boring is the point.

## 3.2 Entry Structure

The base schema (canon_entries, canon_versions, canon_provenance, canon_relations) is defined in the system data model. This module adds typed content per domain.

canon_versions gains an attributes jsonb column validated against a per-domain schema.

- **pricing.** product, plan, amount, currency, billing_period, effective_from, discount_ceiling_percent, approval_threshold.
- **org.** unit, lead, reports_to, headcount, mandate.
- **policy.** applies_to, rule, exceptions, escalation_path.
- **positioning.** audience, claim, proof_points, banned_claims.
- **process.** trigger, steps, owner_role, sla.
- **decision.** question, outcome, reasoning, decided_at, revisit_condition.

statement remains the human-readable sentence, attributes is what the drift engine compares against and what agents consume for precise answers. Domain schemas are versioned JSON Schema documents stored per tenant with platform defaults, validation runs at version creation, invalid attributes reject the write.

## 3.3 Entry Lifecycle

draft, active, decayed, archived.

- Entries are born as drafts from three sources, cold start, drift proposals, and manual creation. A draft becomes active only through the approval flow.
- decayed is automatic, a scheduled job flips active entries whose verified_at plus verify_interval has passed. Decayed entries still serve, labeled canon-stale with downgraded confidence, and generate a decay proposal to the owner.
- archived entries never serve but remain in history, superseded-by relations point at their replacements.

## 3.4 The Approval Transaction

Approving a proposal executes atomically.

1. Insert the new canon_versions row with provenance links to source events.
2. Insert the approvals row (or rows, per policy).
3. Update current_version_id and verified_at, set status active.
4. Resolve the drift proposal.
5. Write the audit entry.

One transaction, all or nothing. Rejection resolves the proposal with a structured rejection reason (wrong, duplicate, not_canon_worthy, bad_draft, other plus note), the taxonomy feeds drift tuning.

Approval requirements come from approval_policies resolved by (tier, domain), most specific wins. Multi-approver policies hold the version in a pending_approval state until the count is met, any rejection cancels.

## 3.5 Bloat Resistance

- **Caps.** Per-tenant entry budget scaled by headcount, defaults 150 entries up to 50 employees, 400 up to 300. Hitting the cap blocks new entries until something merges or archives, by design, and the UI says so plainly.
- **Merge detection.** Weekly job embeds all active statements, pairs above a similarity threshold within the same domain generate a merge proposal to the shared owner, or both owners with the domain lead as tiebreaker.
- **Forced ownership.** Owner departure (HRIS signal) generates reassignment proposals for every owned entry to the departing person's manager. No orphan state exists in the model.

## 3.6 Canon Health Score

Per domain, 0 to 100, surfaced to leadership. Weighted components, share of entries verified within window (40), median proposal resolution time against a 7-day target (25), open contradiction count (20), coverage, gap proposals accepted versus ignored (15). The score exists to make neglect visible, it gates nothing.

---

# Module 04, Drift Engine

## 4.1 Purpose

Detect that the stream contradicts, extends, or outlives the canon, draft the correction, route it to the right human. Precision over recall throughout, owner attention is the scarcest resource in the system.

## 4.2 Tier 1, Candidate Filtering

Runs on every new chunk, target pass rate under 5 percent.

- Similarity against the embedding of every active canon statement in the same tenant, threshold pass if any entry scores above 0.78 cosine.
- Source weighting, CRM and transcript chunks get a threshold discount (more likely to matter), automated messages (CI bots, calendar noise) are hard-excluded by author type.
- Author weighting, chunks authored by an entry's owner or domain leadership pass at a lower threshold.
- Gap path, chunks matching no entry but clustering with other unmatched chunks (rolling HDBSCAN over a 30-day unmatched buffer, per domain-classified topic) become gap candidates when a cluster reaches size and account-diversity thresholds.

Output, (chunk, candidate_entry or candidate_cluster, tier1_score).

## 4.3 Tier 2, Classification

Haiku-class model, one call per candidate, strict JSON output.

Input, the chunk content with source context, the candidate entry's statement and attributes (or cluster digest for gaps). Output schema.

```json
{
  "relation": "contradicts | confirms | extends | unrelated",
  "confidence": 0.0,
  "conflicting_field": "string or null"
}
```

confirms updates the entry's last_referenced_at, which feeds decay. unrelated drops. contradicts and extends above a per-tenant confidence threshold proceed to tier 3. Tier 2 exists to keep tier 3 spend proportional to real signal.

## 4.4 Tier 3, Comparison and Drafting

Frontier model. Input, the full candidate entry (statement, attributes, current version, recent history), the triggering chunk plus its thread context retrieved via thread_key, and up to five corroborating chunks retrieved by similarity. Output, a drafted replacement statement and attributes diff, the minimal supporting excerpts, an explicit contradiction description, and a calibrated confidence.

Calibration is empirical, tier 3 raw confidence is mapped through a per-tenant reliability curve built from the approve and reject history, recalculated weekly. A new tenant starts on the global curve.

## 4.5 Proposal Hygiene

- **Dedup.** One open proposal per (entry, conflicting_field). New evidence attaches to the open proposal rather than spawning siblings.
- **Cooldown.** A rejected proposal's (entry, field) pair is suppressed for 14 days unless evidence volume doubles, in which case it returns flagged as recurring-after-rejection, which is a different and stronger message.
- **Budget.** Per-owner weekly proposal budget, default 10, highest confidence first, the rest queue. An owner drowning in proposals stops reading them, the budget protects the product from itself.
- **Escalation.** Contradictions against bedrock-tier entries route to the domain owner and the admin together, marked strategic.

## 4.6 Decay Scheduler

Daily job. Flips overdue entries to decayed, generates decay proposals, applies last_referenced_at from tier 2 confirms so frequently confirmed entries can auto-extend their window by policy (opt-in per tenant, capped at one extension).

## 4.7 Feedback Loop

Every resolution writes to the tuning store, tier 1 thresholds per source type, tier 2 confidence gates, and the tier 3 calibration curve are all per-tenant parameters adjusted weekly from the rejection taxonomy. wrong lowers trust in the pipeline for that domain, duplicate tightens dedup, not_canon_worthy raises the gap cluster threshold, bad_draft flags prompt regression, not thresholds.

## 4.8 Metrics

Acceptance rate per kind and per domain, target above 70 percent. Tier pass rates against the 5 percent and spend budgets. Time from triggering event to proposal, target under 24 hours. Recurring-after-rejection count, the signal that precision tuning is suppressing truth.

---

# Module 05, Serving Layer

## 5.1 Purpose

One interface for humans and agents. Canon-first answers with receipts, trust labels, and permission enforcement. Agents are first-class principals.

## 5.2 The Answer Contract

Every answer carries three parts. The answer. The receipts, citations resolving to canon entries with approver and verified date, or to stream events. The trust level, exactly one of canon_verified, canon_stale, stream_only, no_coverage. Conflicts are never silently resolved, when canon and open contradictions coexist, both are returned with the conflict flagged.

no_coverage is a first-class answer, not an error.

## 5.3 MCP Tools

**query**

```json
{
  "question": "string",
  "domains": ["pricing"],
  "include_stream": true,
  "max_citations": 5
}
```

Response.

```json
{
  "answer": "string",
  "trust": "canon_verified",
  "citations": [
    {
      "type": "canon",
      "entry_id": "uuid",
      "version": 4,
      "verified_at": "2026-06-20",
      "approver": "person:uuid",
      "statement": "string"
    }
  ],
  "conflicts": [
    { "entry_id": "uuid", "description": "string", "proposal_id": "uuid" }
  ],
  "freshness": { "oldest_citation": "2026-06-20", "decayed_entries_used": 0 }
}
```

**get_entry.** Entry id in, full entry with attributes, version history, provenance links, relations out. **list_conflicts.** Open contradictions visible to the caller, filterable by domain. **propose_update.** Agents submit proposals into the same owner pipeline, marked agent-originated, subject to the same budget and hygiene.

Retrieval order inside query. Canon relational lookup filtered by domain and visibility, attributes answered directly when the question maps to typed fields, statement synthesis otherwise. Stream fallback only when include_stream and canon coverage is insufficient, always labeled. Synthesis model never receives content the caller's ACL does not admit, enforcement is in retrieval, not in the prompt.

## 5.4 Principals, Credentials, Limits

Agents get scoped API keys bound to a people row with role agent, a visibility scope, allowed domains, and a rate tier. Keys are revocable individually, every call audits (actor, tool, entry ids touched, trust level served). Rate limits per key with burst allowance, metering events emitted per billable query for the billing pipeline.

## 5.5 Error Taxonomy

- 401 unknown or revoked key. 403 scope violation, the response never confirms whether the forbidden thing exists. 404 only for ids the caller could see if they existed. 422 malformed request with field errors. 429 rate limited with retry-after. 503 degraded retrieval, canon-only answers still served when the vector path is down, labeled accordingly.

REST mirrors every MCP tool one-to-one for non-MCP consumers.

---

# Module 06, Cold Start

## 6.1 Purpose

From connected tools to a reviewed, living canon in under a day. Onboarding, demo, and sales pitch in one pipeline.

## 6.2 Pipeline

1. Backfill (module 01) streams 90 days, most recent first. Cold start consumes chunks as they land, it does not wait for completion.
2. **Org inference.** HRIS when connected is authoritative. Otherwise inferred from email domains, Slack channel leadership signals, and signature blocks, marked inferred and drafted as org entries at lower confidence.
3. **Decision mining.** Tier 3 model over transcript and thread chunks matching decision-language patterns pre-filtered by tier 1, drafted as decision entries.
4. **Pricing and policy signals.** CRM and email chunks clustered per the gap path, drafted into pricing and policy domains.
5. **Wiki import.** Notion and Confluence pages ranked by recency and inbound references, top pages summarized into candidate entries with the page as provenance.
6. Everything lands as draft entries in a prioritized review queue, bedrock candidates first, then pricing, then the rest by confidence.

## 6.3 Review Experience Contract

The founder reviews in a dedicated flow, approve, edit, reject per entry, batch operations by domain, progress bar against the entry budget. Target, a 100-person company reviews the draft canon in one afternoon. Cold start inference cost is budgeted per company size and monitored per tenant, 200 to 500 dollars, it is the pilot's cost of goods.

---

# Module 07, Web App and Slack App

## 7.1 Web App Surfaces (Next.js)

- **Canon browser.** By domain, entry detail with statement, attributes, version timeline, provenance graph, relations. Search across canon.
- **Approval queue.** Unified queue of drift, merge, reassignment, and cold-start items scoped to the viewer's ownership, single-action resolution, diff view of statement and attributes changes, source excerpts one click deep.
- **Drift dashboard.** Open contradictions, acceptance trends, recurring-after-rejection flags, per-domain health scores.
- **Connector management.** Status, lag, backfill progress, exclusion rule configuration, degraded-state remediation.
- **Audit log.** Filterable by actor, action, subject, exportable on Scale tier.
- **Admin.** People and roles, approval policies, agent key issuance and revocation, entry budget, tenant settings.

Frontend standards. TypeScript strict, server components by default, client components only where interaction demands, Tailwind utility classes with a shared design token layer, all list and diff views built from shared reusable components, no view-specific one-off variants.

## 7.2 Slack App

Mirrors the approval queue only. Proposal notifications as messages with approve, edit, reject actions, edit opens a modal, decisions round-trip through the same approval transaction as the web app, one code path. Daily digest option instead of per-proposal messages. Nothing else lives in Slack, governance in-flow, browsing on the web.

---

# Module 08, Security, Tenancy, Audit, Billing, Observability

## 8.1 Tenancy and Data Protection

- Row-level security on every table, session sets app.tenant_id, policies enforce it, verified by an automated test that attempts cross-tenant reads through every API path on every deploy.
- Per-tenant data keys encrypting object-storage payloads, wrapped by a KMS master key, rotation supported by re-wrapping without re-encrypting payloads.
- Tenant offboarding, scheduled hard-delete of payloads, events, chunks, and canon after a grace window, key destruction as the final act, certificate of deletion generated from the audit trail.
- Verified erasure requests (a person, not a tenant), tombstone plus payload deletion plus chunk removal, canon entries citing erased events keep the entry with provenance marked erased.

## 8.2 Identity and Access

SSO (OIDC and SAML) and SCIM provisioning from day one. Roles admin, owner, member, agent as defined in the data model. Agent keys per module 05. Web sessions and API keys share one authorization layer, permission checks live in one package consumed by every service.

## 8.3 Audit

Every state change and every served answer writes an audit row, partitioned monthly, append-only, no update or delete grants exist on the table for any application role. Scale tier gets scheduled exports. SOC 2 posture from the start, the audit table, RLS tests, key management, and access reviews are the evidence backbone.

## 8.4 Billing

Metering events from the serving layer (billable agent queries) flow through the queue into a metering table, aggregated daily, pushed to Stripe subscription items. Platform fee as the base subscription per tier. Included query volume as a tier attribute checked at aggregation, overage billed per thousand queries. Grace behavior, overage never hard-stops queries mid-month, it bills, hard caps are an opt-in tenant setting.

## 8.5 Observability

- One trace id per request across serving, retrieval, and synthesis, one per event across ingestion, embedding, and drift.
- Golden signals per service, plus the product metrics defined in each module.
- Cost telemetry, inference spend per tenant per tier of the drift cascade, cold start spend per onboarding, alert on tenants exceeding cost model assumptions by 2x.
- Status page fed by connector health and serving availability.

---


---

# Module 10, UI/UX Design

## 10.1 Design Thesis

The product sells provable trust, so the interface must feel like an instrument, not an app. The reference points are mission control and a private bank, precision surfaces, calm density, information that earns its place. Dark mode is the only mode, not a preference toggle, because the product is ambient infrastructure that people glance at between real work, and a dark instrument panel reads as always-on in a way a white document page never will.

The single idea every screen expresses, truth has a receipt. Verified knowledge glows warm, unverified signal stays cool and recedes, and the path from any statement back to its human approver is always one gesture away.

One deliberate rejection. The default futuristic look, near-black with one acid-green accent, is what every AI dashboard ships. This product uses warmth as its signal instead, because the emotional job of the interface is trust, and trust is warm.

## 10.2 Color System

Base surfaces, a deep blue-black ramp, never pure black, pure black makes elevated panels impossible.

- void, #0A0D14, the app background
- surface, #10141D, cards and panels
- surface-raised, #161B27, modals, popovers, hovered rows
- line, #232A38, hairline borders and dividers
- line-strong, #303948, focused and active borders

Text.

- text-primary, #E8EAF0
- text-secondary, #9AA3B5
- text-muted, #5E687C, timestamps, metadata, never for content

The trust spectrum, the heart of the system. Every piece of knowledge on screen carries exactly one of these, and the colors are reserved, nothing else on the interface may use them.

- verified, #E8B04B, warm amber gold. Canon-verified content, approval seals, the receipt glyph. The only warm color in the interface, which is precisely why it means what it means.
- stale, #A08850, desaturated ochre. Decayed canon, served but flagged.
- stream, #6B87A8, cool steel blue. Stream-only content, unverified signal.
- conflict, #E5484D, signal red. Open contradictions, and nothing else, not errors, not destructive buttons, those get their own muted treatment, red is spent entirely on drift.
- none, #5E687C, no-coverage states.

Functional accents, used sparingly.

- action, #7B9EF8, a restrained periwinkle for primary interactive elements, links, and focus rings
- positive, #4CAF8E, success confirmations only

## 10.3 Typography

Three faces, three jobs.

- **Display, Archivo Expanded.** Page titles, domain headers, the big numbers on the drift dashboard. Wide, industrial, set tight, weights 600 and 800 only. This face carries the futuristic register so nothing else has to.
- **Body, Instrument Sans.** Everything readable, 400 and 500, 15px base on desktop, 1.6 line height. Quiet on purpose.
- **Data, JetBrains Mono.** Timestamps, ids, confidence values, version numbers, excerpts from source events, and the entire receipt component. The mono face is the visual marker for evidence, when the reader sees mono, they are looking at ground-level fact, not interface chrome.

Type scale, 13, 15, 17, 22, 28, 40. Eyebrow labels in 11px mono uppercase with 8 percent letter spacing, used for domain tags and trust labels.

## 10.4 The Signature, the Receipt

Every canon-backed answer, entry, and citation carries the receipt, a compact horizontal chain rendered in mono, seal glyph, approver, verified date, source count.

Collapsed it reads as a single quiet line under the statement. Expanded, one click or the R key, it unfolds into a vertical provenance chain, entry, version, approval, source events, each node a link, connected by a thin amber thread on the void background. The thread is drawn with a 300ms trace animation on expand, the one place the interface performs, because watching the line of trust draw itself from statement to source is the product's entire pitch in half a second.

The receipt is one component. The approval queue, canon browser, version history, serving previews, and cold-start review all render it. There is exactly one of it.

## 10.5 Layout System

- Fixed left rail, 240px, navigation by surface, canon, queue, drift, connectors, audit, admin. Collapsed rail at 64px shows glyphs only.
- Content area on an 8px spacing grid, max content width 1200px for reading surfaces, full-bleed for the dashboard.
- Density is a virtue. Rows at 44px, generous only where reading happens, entry statements and diffs get air, metadata stays compact.
- Elevation by surface color step plus a 1px line border, never drop shadows heavier than 0 4px 24px rgba(0,0,0,0.4). Glow is reserved for the trust spectrum, a verified seal may glow faintly, a button may not.
- Corner radius 6px on controls, 10px on cards, nothing fully rounded except status dots and avatars.

## 10.6 Motion

Motion explains state change, never decorates.

- Standard transitions 150ms ease-out, surface enter and exit 200ms with 4px translate.
- The receipt trace, 300ms, described above.
- The drift pulse, the one ambient element. On the dashboard, each domain header carries a thin baseline that pulses once, subtly, when new stream events arrive in that domain, a heartbeat of maybe 2 seconds, opacity 0.3 to 0.6. The brain is alive and the interface breathes exactly this much and no more.
- Approval actions resolve optimistically with a 150ms seal-stamp scale on the verified glyph.
- prefers-reduced-motion disables the trace, the pulse, and all translates, opacity-only fallbacks.

## 10.7 Surface-by-Surface UX

**Canon browser.** Left, domain list with per-domain health rings, a thin circular gauge in the trust spectrum colors. Center, entry list, each row shows statement, owner avatar, trust dot, verified date in mono. Right, the detail pane, statement large in body face, attributes as a mono key-value block, the receipt, the version timeline as a vertical thread of amber nodes, relations as chips. Search is a command palette, cmd-K, spanning canon first, stream results below a hard visual divider in cool blue, the two-layer architecture visible in the search results themselves.

**Approval queue.** The sacred surface, designed for velocity. One proposal at a time in a focused center column, kind and confidence in the eyebrow, the diff as the centerpiece, removed text struck in muted red-gray, added text in amber, attributes diff as a mono block. Source excerpts dock on the right, cool blue border coding them as stream. Keyboard-first, A approve, E edit, X reject with a reason picker, J and K to move. A session progress rail on the left edge fills as the owner clears the queue, gamified exactly this much and no more. Batch mode collapses to a table with multi-select.

**Drift dashboard.** The mission-control surface and the demo screen. Top band, three display-face numbers, open contradictions, pending proposals, canon health, each with a seven-day sparkline in mono-thin strokes. Center, the domain grid, each domain a card with its health ring, pulse baseline, and open-item count. Conflict cards surface below in signal red left-borders, statement versus stream claim side by side. This screen must look extraordinary in a sales demo at 1080p, it is the poster.

**Cold start review.** A guided full-screen flow, progress bar against the entry budget across the top, one drafted entry at a time using the same diff and receipt components, domain-batched with a domain intro screen showing what was inferred and from how many events. The final screen renders the founder's new canon as a constellation, entries as amber points grouped by domain on the void, the moment the company sees itself as a machine-readable thing for the first time. Screenshot-bait, deliberately.

**Connector management.** Status board, each connector a card with source glyph, state (live in positive, degraded in ochre with remediation steps inline, backfilling with a determinate progress bar), lag figure in mono. Exclusion rule editor as plain readable rules, no regex exposed unless the user opens advanced.

**Audit log.** Dense mono table, filter bar pinned, every row expandable to the full record. This surface is allowed to look like a terminal, that is its honesty.

**Empty, loading, error.** Empty states name the next action, an empty queue says the canon is current and shows the last cleared item. Loading is skeleton rows in surface-raised, never spinners on content surfaces. Errors state what happened and the remediation, in the interface's voice, no apologies, no mascots.

## 10.8 Slack Surface

Slack styling is not controllable, so the design system reduces to language and structure, proposal messages lead with the kind and the entry statement, the diff as a quote block, three buttons in the fixed order approve, edit, reject, and the receipt collapsed to one mono-style context line, approver, date, source count. Identical vocabulary to the web app, an action called Approve produces a confirmation that says Approved.

## 10.9 Copy Voice

Plain verbs, sentence case, no filler. Buttons say what happens, Approve entry, Reject with reason, Reconnect Slack. Trust labels are exact and consistent everywhere, Verified, Stale, Stream signal, No coverage, never synonyms. The interface never says AI thinks or AI found, it says detected in 12 events or contradicts pricing entry, evidence has counts and sources, not feelings. Numbers in mono, always.

## 10.10 Accessibility Floor

Contrast 4.5 to 1 minimum for text on all surfaces, the trust spectrum values are chosen to pass on void and surface. Trust is never color-alone, every trust state pairs its color with its glyph and label. Full keyboard traversal on every surface with visible focus rings in action blue, the approval queue is operable without a pointer entirely. Reduced motion honored per 10.6. Semantic landmarks and live-region announcements for queue advancement and toast confirmations.

## 10.11 Implementation Notes

Tokens ship as the Tailwind theme in apps/web, colors, radii, type scale, spacing, motion durations, named exactly as in this document. Components consume tokens only, a raw hex in a class string fails review. The receipt, trust badge, diff view, health ring, and pulse baseline are shared primitives in the component library before any surface is assembled from them. Fonts self-hosted, variable where available, display face subset to the weights used.


---

# Build Order Across Modules

1. Module 01 spine plus Slack connector.
2. Module 02 pipeline and retrieval.
3. Module 03 canon, approvals, policies.
4. Module 05 serving, canon-only answers first.
5. Module 04 drift engine, tier 1 through 3, feedback loop.
6. Module 06 cold start.
7. Remaining connectors in module 01 priority order.
8. Module 07 surfaces harden throughout, approval queue lands with module 03.
9. Module 08 runs from day one, RLS, audit, and keys are not retrofittable.

The dependency logic. Serving before drift because drift proposals need somewhere to land and something to compare against. Cold start after drift because it reuses the cascade. Security first because the two invariants and RLS are foundations, not features.
-e 

---


## 9.1 Hosting Posture

Managed-first, boring-first. The product's risk budget is spent on permissions and drift precision, not on operating databases.

- **Compute.** Containerized services on a managed platform (Fly.io or Railway to start, ECS when a tenant demands it). Four deployables, ingestion (Go), embedding workers (Go), serving API (NestJS), web (Next.js on Vercel or same platform). Drift engine runs as a worker pool inside the serving deployable at first, extracted when its scaling profile diverges.
- **Database.** Managed Postgres with pgvector (Supabase, Neon, or RDS). Requirements that decide the pick, point-in-time recovery, logical replication for future read replicas, per-database extensions control, connection pooling built in.
- **Queue.** Postgres-backed queue to start (single infra dependency, transactional enqueue with the event write). Migration path to SQS or NATS documented, triggered when queue depth p99 or throughput crosses defined thresholds, not before.
- **Object storage.** S3 or R2, per-tenant prefix, per-tenant KMS-wrapped data keys as specified in module 08.
- **Region.** Single region to start. Multi-region is a Scale-tier conversation, not a launch requirement.

## 9.2 Environments

Three, no more.

- **local.** Docker compose, Postgres plus MinIO plus the services. Seed script creates one tenant, two connectors with fixture data, a populated canon. One command to a working brain.
- **staging.** Full stack, real integrations against sandbox workspaces (a dedicated Slack workspace, a test Google Workspace). Every merge to main deploys here automatically.
- **production.** Deploys by promotion of the exact staging artifact, never a rebuild.

No per-developer cloud environments. Local covers development, staging covers integration.

## 9.3 CI/CD

Pipeline on every PR.

1. Lint and typecheck, Go vet and staticcheck, TypeScript strict, no warnings tolerated.
2. Unit tests per service.
3. The RLS cross-tenant test suite from module 08, a failed isolation test blocks merge unconditionally.
4. Connector contract tests against recorded fixtures.
5. Migration dry-run against a snapshot of the staging schema.

On merge to main, build once, deploy to staging, run smoke suite (ingest a fixture event end to end, query it through serving, verify the trust label). Production promotion is a manual approval on the same artifact. Rollback is redeploying the previous artifact, migrations are forward-only, see 9.4.

## 9.4 Migrations

- One migrations directory, sequential, applied by a migration runner as a deploy step before the new artifact serves traffic.
- Forward-only. Rollback of code is safe because every migration must be backward-compatible with the previous application version, expand-and-contract pattern, add the new column, deploy code that writes both, backfill, deploy code that reads new, drop the old column in a later migration.
- Destructive operations (drop, truncate, delete) require a second reviewer on the PR and never ship in the same release as the code depending on them.
- Partitions (events, event_chunks, audit_log) are created ahead by a scheduled job, three months forward, never on the write path.

## 9.5 Secrets and Config

- Platform secret manager for service credentials, injected as environment at deploy, never in the repo, never in images.
- Tenant-level secrets (connector OAuth tokens) live encrypted in the connectors config column under the tenant data key, not in the platform secret store, they are data, not deploy config.
- Config is env-var driven with a single typed config package per service, fail-fast on missing values at boot, no runtime defaults for anything security-relevant.

## 9.6 Model Provider Configuration

- Anthropic API for tier 2, tier 3, and serving synthesis, keys per environment, spend alerts per key.
- Embedding provider behind the module 02 interface, provider choice is config, not code.
- Per-tenant spend tracking from day one via the cost telemetry in module 08, the drift cascade budget assumptions are enforced by alerting, not hoped.

## 9.7 Backups and DR

- Continuous PITR on Postgres, 30-day window.
- Object storage versioning on, lifecycle rules per retention policy.
- Quarterly restore drill, restore staging from a production backup into an isolated environment, run the smoke suite, record time-to-restore. A backup that has never been restored is a rumor.
- RTO 4 hours, RPO 5 minutes, written down so Scale-tier security reviews have an answer.

## 9.8 Domains and Edge

- api.<domain> for REST and MCP, app.<domain> for web, hooks.<domain> for webhook receipt, isolated so a webhook flood never degrades serving.
- TLS everywhere, HSTS, webhook endpoints verify source signatures (Slack signing secret, Google channel tokens) before anything touches the queue.

## 9.9 Week-One Checklist

1. Repo scaffolding, monorepo, one package per service, shared types package, lint and CI green on empty services.
2. Managed Postgres provisioned, migration runner wired, initial schema applied, RLS policies plus the isolation test suite passing.
3. Object storage bucket, KMS key, tenant-key wrapping utility.
4. Docker compose local environment with seed script.
5. Staging deploy pipeline live, hello-world serving endpoint behind TLS.
6. Then module 01, section 1.10, step 1.
