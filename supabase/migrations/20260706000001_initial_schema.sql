create extension if not exists vector with schema extensions;

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
  id uuid not null default gen_random_uuid(),
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
  primary key (id, occurred_at),
  unique (tenant_id, connector_id, external_id, occurred_at)
) partition by range (occurred_at);

create table event_chunks (
  id uuid not null default gen_random_uuid(),
  tenant_id uuid not null references tenants(id),
  event_id uuid not null,
  event_occurred_at timestamptz not null,
  chunk_index int not null,
  content text not null,
  embedding extensions.vector(1536) not null,
  embedding_model text not null,
  acl jsonb not null,
  created_at timestamptz not null default now(),
  primary key (id, created_at),
  foreign key (event_id, event_occurred_at) references events(id, occurred_at)
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
  tenant_id uuid not null references tenants(id),
  entry_id uuid not null references canon_entries(id),
  version_number int not null,
  statement text not null,
  created_by uuid not null references people(id),
  created_at timestamptz not null default now(),
  unique (entry_id, version_number)
);

alter table canon_entries
  add constraint canon_entries_current_version_fk
  foreign key (current_version_id) references canon_versions(id);

create table canon_provenance (
  tenant_id uuid not null references tenants(id),
  version_id uuid not null references canon_versions(id),
  event_id uuid not null,
  event_occurred_at timestamptz not null,
  primary key (version_id, event_id),
  foreign key (event_id, event_occurred_at) references events(id, occurred_at)
);

create table canon_relations (
  tenant_id uuid not null references tenants(id),
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
  tenant_id uuid not null references tenants(id),
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
  id bigint generated always as identity,
  tenant_id uuid not null references tenants(id),
  actor_id uuid references people(id),
  action text not null,
  subject_type text not null,
  subject_id uuid,
  occurred_at timestamptz not null default now(),
  primary key (id, occurred_at)
) partition by range (occurred_at);

create index events_tenant_occurred_idx on events (tenant_id, occurred_at desc);
create index events_connector_idx on events (connector_id);
create index events_author_idx on events (author_id);
create index events_thread_idx on events (tenant_id, thread_key) where thread_key is not null;
create index event_chunks_tenant_idx on event_chunks (tenant_id);
create index event_chunks_event_idx on event_chunks (event_id);
create index event_chunks_embedding_idx on event_chunks
  using hnsw (embedding extensions.vector_cosine_ops);
create index people_tenant_idx on people (tenant_id);
create index connectors_tenant_idx on connectors (tenant_id);
create index canon_entries_tenant_domain_idx on canon_entries (tenant_id, domain);
create index canon_entries_owner_idx on canon_entries (owner_id);
create index canon_versions_entry_idx on canon_versions (entry_id);
create index canon_versions_tenant_idx on canon_versions (tenant_id);
create index canon_provenance_tenant_idx on canon_provenance (tenant_id);
create index canon_provenance_event_idx on canon_provenance (event_id);
create index canon_relations_tenant_idx on canon_relations (tenant_id);
create index canon_relations_to_idx on canon_relations (to_entry);
create index approval_policies_tenant_idx on approval_policies (tenant_id);
create index approvals_tenant_idx on approvals (tenant_id);
create index approvals_version_idx on approvals (version_id);
create index approvals_approver_idx on approvals (approver_id);
create index drift_proposals_tenant_status_idx on drift_proposals (tenant_id, status);
create index drift_proposals_entry_idx on drift_proposals (entry_id);
create index drift_proposals_routed_idx on drift_proposals (routed_to);
create index audit_log_tenant_occurred_idx on audit_log (tenant_id, occurred_at desc);
create index audit_log_actor_idx on audit_log (actor_id);

create function public.create_monthly_partitions(
  parent regclass,
  from_month date,
  months_ahead int
) returns void
language plpgsql
as $$
declare
  month_start date;
  partition_name text;
begin
  for offset_months in 0..months_ahead loop
    month_start := date_trunc('month', from_month) + make_interval(months => offset_months);
    partition_name := format(
      '%s_%s',
      replace(parent::text, 'public.', ''),
      to_char(month_start, 'YYYY_MM')
    );
    execute format(
      'create table if not exists %I partition of %s for values from (%L) to (%L)',
      partition_name,
      parent,
      month_start,
      month_start + interval '1 month'
    );
  end loop;
end;
$$;

select public.create_monthly_partitions('events', current_date, 3);
select public.create_monthly_partitions('event_chunks', current_date, 3);
select public.create_monthly_partitions('audit_log', current_date, 3);
