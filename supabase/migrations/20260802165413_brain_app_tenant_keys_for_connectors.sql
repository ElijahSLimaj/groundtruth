grant select, insert on tenant_keys to brain_app;

create policy tenant_read on tenant_keys
  for select to brain_app
  using (tenant_id = public.app_tenant_id());

create policy tenant_create on tenant_keys
  for insert to brain_app
  with check (tenant_id = public.app_tenant_id());
