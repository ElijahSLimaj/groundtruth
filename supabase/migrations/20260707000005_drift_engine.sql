alter table canon_entries add column last_referenced_at timestamptz;

alter table drift_proposals drop constraint drift_proposals_kind_check;
alter table drift_proposals add constraint drift_proposals_kind_check
  check (kind in ('contradiction', 'extension', 'gap', 'decay'));
alter table drift_proposals add column conflicting_field text;
alter table drift_proposals add column strategic boolean not null default false;
alter table drift_proposals add column escalated_to uuid references people(id);
alter table drift_proposals add column recurring_after_rejection boolean not null default false;
alter table drift_proposals add constraint drift_proposals_status_check
  check (status in ('pending', 'queued', 'resolved'));

create index drift_proposals_open_field_idx
  on drift_proposals (tenant_id, entry_id, conflicting_field)
  where status in ('pending', 'queued');

create table drift_evidence (
  id bigint generated always as identity primary key,
  tenant_id uuid not null references tenants(id),
  proposal_id uuid not null references drift_proposals(id),
  event_id uuid not null,
  chunk_id uuid not null,
  added_at timestamptz not null default now()
);

create index drift_evidence_proposal_idx on drift_evidence (proposal_id);

create table drift_state (
  tenant_id uuid primary key references tenants(id),
  last_chunk_created_at timestamptz not null default 'epoch',
  last_chunk_id uuid not null default '00000000-0000-0000-0000-000000000000',
  updated_at timestamptz not null default now()
);

create table drift_tuning (
  tenant_id uuid primary key references tenants(id),
  params jsonb not null default '{}',
  updated_at timestamptz not null default now()
);

create table canon_statement_embeddings (
  version_id uuid not null references canon_versions(id),
  embedding_model text not null,
  tenant_id uuid not null references tenants(id),
  embedding extensions.vector(1536) not null,
  created_at timestamptz not null default now(),
  primary key (version_id, embedding_model)
);

create index canon_statement_embeddings_tenant_idx on canon_statement_embeddings (tenant_id);

grant select, insert, update, delete on drift_evidence, drift_state, drift_tuning to brain_app;
grant select on canon_statement_embeddings to brain_app;
grant select, insert, delete on canon_statement_embeddings to brain_embedder;
grant select on canon_versions, canon_entries to brain_embedder;
grant usage on all sequences in schema public to brain_app;

alter table drift_evidence enable row level security;
alter table drift_state enable row level security;
alter table drift_tuning enable row level security;
alter table canon_statement_embeddings enable row level security;

create policy tenant_isolation on drift_evidence
  for all to brain_app
  using (tenant_id = public.app_tenant_id())
  with check (tenant_id = public.app_tenant_id());

create policy tenant_isolation on drift_state
  for all to brain_app
  using (tenant_id = public.app_tenant_id())
  with check (tenant_id = public.app_tenant_id());

create policy tenant_isolation on drift_tuning
  for all to brain_app
  using (tenant_id = public.app_tenant_id())
  with check (tenant_id = public.app_tenant_id());

create policy tenant_isolation on canon_statement_embeddings
  for select to brain_app
  using (tenant_id = public.app_tenant_id());

create policy embedder_all on canon_statement_embeddings
  for all to brain_embedder
  using (true)
  with check (true);

create policy embedder_read_versions on canon_versions
  for select to brain_embedder
  using (true);

create policy embedder_read_entries on canon_entries
  for select to brain_embedder
  using (true);
