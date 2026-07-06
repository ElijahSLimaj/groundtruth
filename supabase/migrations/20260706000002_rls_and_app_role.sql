do $$
begin
  if not exists (select from pg_roles where rolname = 'brain_app') then
    create role brain_app nologin;
  end if;
end;
$$;

grant brain_app to postgres;

create function public.app_tenant_id() returns uuid
language sql
stable
as $$
  select nullif(current_setting('app.tenant_id', true), '')::uuid
$$;

grant usage on schema public to brain_app;
grant usage on schema extensions to brain_app;
grant execute on function public.app_tenant_id() to brain_app;
grant usage on all sequences in schema public to brain_app;

grant select on tenants to brain_app;
grant select, insert, update on people to brain_app;
grant select, insert, update on connectors to brain_app;
grant select, insert on events to brain_app;
grant select, insert, delete on event_chunks to brain_app;
grant select, insert, update on canon_entries to brain_app;
grant select, insert on canon_versions to brain_app;
grant select, insert on canon_provenance to brain_app;
grant select, insert, delete on canon_relations to brain_app;
grant select, insert, update, delete on approval_policies to brain_app;
grant select, insert on approvals to brain_app;
grant select, insert, update on drift_proposals to brain_app;
grant select, insert on audit_log to brain_app;

alter table tenants enable row level security;
alter table people enable row level security;
alter table connectors enable row level security;
alter table events enable row level security;
alter table event_chunks enable row level security;
alter table canon_entries enable row level security;
alter table canon_versions enable row level security;
alter table canon_provenance enable row level security;
alter table canon_relations enable row level security;
alter table approval_policies enable row level security;
alter table approvals enable row level security;
alter table drift_proposals enable row level security;
alter table audit_log enable row level security;

create policy tenant_isolation on tenants
  for select to brain_app
  using (id = public.app_tenant_id());

create policy tenant_isolation on people
  for all to brain_app
  using (tenant_id = public.app_tenant_id())
  with check (tenant_id = public.app_tenant_id());

create policy tenant_isolation on connectors
  for all to brain_app
  using (tenant_id = public.app_tenant_id())
  with check (tenant_id = public.app_tenant_id());

create policy tenant_isolation on events
  for all to brain_app
  using (tenant_id = public.app_tenant_id())
  with check (tenant_id = public.app_tenant_id());

create policy tenant_isolation on event_chunks
  for all to brain_app
  using (tenant_id = public.app_tenant_id())
  with check (tenant_id = public.app_tenant_id());

create policy tenant_isolation on canon_entries
  for all to brain_app
  using (tenant_id = public.app_tenant_id())
  with check (tenant_id = public.app_tenant_id());

create policy tenant_isolation on canon_versions
  for all to brain_app
  using (tenant_id = public.app_tenant_id())
  with check (tenant_id = public.app_tenant_id());

create policy tenant_isolation on canon_provenance
  for all to brain_app
  using (tenant_id = public.app_tenant_id())
  with check (tenant_id = public.app_tenant_id());

create policy tenant_isolation on canon_relations
  for all to brain_app
  using (tenant_id = public.app_tenant_id())
  with check (tenant_id = public.app_tenant_id());

create policy tenant_isolation on approval_policies
  for all to brain_app
  using (tenant_id = public.app_tenant_id())
  with check (tenant_id = public.app_tenant_id());

create policy tenant_isolation on approvals
  for all to brain_app
  using (tenant_id = public.app_tenant_id())
  with check (tenant_id = public.app_tenant_id());

create policy tenant_isolation on drift_proposals
  for all to brain_app
  using (tenant_id = public.app_tenant_id())
  with check (tenant_id = public.app_tenant_id());

create policy tenant_isolation on audit_log
  for all to brain_app
  using (tenant_id = public.app_tenant_id())
  with check (tenant_id = public.app_tenant_id());
