create table ingestion_queue (
  id bigint generated always as identity primary key,
  tenant_id uuid not null references tenants(id),
  event jsonb not null,
  attempts int not null default 0,
  next_attempt_at timestamptz not null default now(),
  enqueued_at timestamptz not null default now(),
  last_error text
);

create index ingestion_queue_ready_idx on ingestion_queue (next_attempt_at, id);

create table ingestion_dlq (
  id bigint generated always as identity primary key,
  tenant_id uuid not null references tenants(id),
  event jsonb not null,
  reason text not null,
  attempts int not null default 0,
  enqueued_at timestamptz not null,
  failed_at timestamptz not null default now(),
  replayed_at timestamptz
);

create index ingestion_dlq_tenant_idx on ingestion_dlq (tenant_id);

do $$
begin
  if not exists (select from pg_roles where rolname = 'brain_worker') then
    create role brain_worker nologin;
  end if;
end;
$$;

grant brain_worker to postgres;

grant usage on schema public to brain_worker;
grant usage on schema extensions to brain_worker;
grant execute on function public.app_tenant_id() to brain_worker;
grant usage on all sequences in schema public to brain_worker;

grant select, insert, update, delete on ingestion_queue to brain_worker;
grant select, insert on ingestion_dlq to brain_worker;
grant insert on events to brain_worker;

alter table ingestion_queue enable row level security;
alter table ingestion_dlq enable row level security;

create policy worker_all_tenants on ingestion_queue
  for all to brain_worker
  using (true)
  with check (true);

create policy worker_all_tenants on ingestion_dlq
  for all to brain_worker
  using (true)
  with check (true);

create policy worker_tenant_write on events
  for insert to brain_worker
  with check (tenant_id = public.app_tenant_id());
