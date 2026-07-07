create table metering_events (
  id bigint generated always as identity primary key,
  tenant_id uuid not null references tenants(id),
  api_key_id uuid references api_keys(id),
  person_id uuid references people(id),
  tool text not null,
  occurred_at timestamptz not null default now()
);

create index metering_events_tenant_day_idx on metering_events (tenant_id, occurred_at);

grant select, insert on metering_events to brain_app;

alter table metering_events enable row level security;

create policy tenant_isolation on metering_events
  for all to brain_app
  using (tenant_id = public.app_tenant_id())
  with check (tenant_id = public.app_tenant_id());

create view metering_daily
with (security_invoker = true) as
select tenant_id,
       date_trunc('day', occurred_at) as day,
       tool,
       count(*) as calls
from metering_events
group by tenant_id, date_trunc('day', occurred_at), tool;

grant select on metering_daily to brain_app;

create function public.list_tenant_ids() returns setof uuid
language sql
stable
security definer
set search_path = public
as $$
  select id from tenants order by created_at
$$;

revoke all on function public.list_tenant_ids from public;
grant execute on function public.list_tenant_ids to brain_app;
