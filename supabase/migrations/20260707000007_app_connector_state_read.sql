grant select on connector_state to brain_app;

create policy tenant_read on connector_state
  for select to brain_app
  using (tenant_id = public.app_tenant_id());
