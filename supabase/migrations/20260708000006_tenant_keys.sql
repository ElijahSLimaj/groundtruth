create table tenant_keys (
  tenant_id uuid primary key references tenants(id),
  wrapped_key bytea not null,
  key_version int not null default 1,
  created_at timestamptz not null default now(),
  rewrapped_at timestamptz
);

grant select, insert, update on tenant_keys to brain_worker;

alter table tenant_keys enable row level security;

create policy worker_all_tenants on tenant_keys
  for all to brain_worker
  using (true)
  with check (true);
