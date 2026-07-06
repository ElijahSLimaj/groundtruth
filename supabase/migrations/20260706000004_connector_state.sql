create table connector_state (
  connector_id uuid primary key references connectors(id),
  tenant_id uuid not null references tenants(id),
  poll_cursor text not null,
  updated_at timestamptz not null default now()
);

create index connector_state_tenant_idx on connector_state (tenant_id);

grant select, insert, update, delete on connector_state to brain_worker;
grant select on connectors to brain_worker;
grant update (status) on connectors to brain_worker;

alter table connector_state enable row level security;

create policy worker_all_tenants on connector_state
  for all to brain_worker
  using (true)
  with check (true);

create policy worker_read_all on connectors
  for select to brain_worker
  using (true);

create policy worker_status_update on connectors
  for update to brain_worker
  using (true)
  with check (true);
