create table cold_start_state (
  tenant_id uuid primary key references tenants(id),
  last_chunk_created_at timestamptz not null default 'epoch',
  last_chunk_id uuid not null default '00000000-0000-0000-0000-000000000000',
  llm_calls int not null default 0,
  updated_at timestamptz not null default now()
);

grant select, insert, update, delete on cold_start_state to brain_app;

alter table cold_start_state enable row level security;

create policy tenant_isolation on cold_start_state
  for all to brain_app
  using (tenant_id = public.app_tenant_id())
  with check (tenant_id = public.app_tenant_id());
