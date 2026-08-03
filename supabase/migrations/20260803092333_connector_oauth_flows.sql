create table oauth_flows (
  state text primary key,
  tenant_id uuid not null references tenants(id),
  person_id uuid not null references people(id),
  source_type text not null,
  code_verifier text not null,
  redirect_uri text not null,
  created_at timestamptz not null default now()
);

create index oauth_flows_created_idx on oauth_flows (created_at);

grant insert on oauth_flows to brain_app;

alter table oauth_flows enable row level security;

create policy tenant_create on oauth_flows
  for insert to brain_app
  with check (tenant_id = public.app_tenant_id());

create function public.oauth_flow_consume(p_state text)
returns table (
  tenant_id uuid,
  person_id uuid,
  source_type text,
  code_verifier text,
  redirect_uri text
)
language plpgsql
security definer
set search_path = public
as $$
begin
  return query
  delete from oauth_flows f
  where f.state = p_state
    and f.created_at > now() - interval '10 minutes'
  returning f.tenant_id, f.person_id, f.source_type, f.code_verifier, f.redirect_uri;
end;
$$;

revoke all on function public.oauth_flow_consume(text) from public;
grant execute on function public.oauth_flow_consume(text) to brain_app;

alter table connectors
  add constraint connectors_tenant_source_unique unique (tenant_id, source_type);
