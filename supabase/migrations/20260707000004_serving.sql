create extension if not exists pgcrypto with schema extensions;

create table api_keys (
  id uuid primary key default gen_random_uuid(),
  tenant_id uuid not null references tenants(id),
  person_id uuid not null references people(id),
  key_hash text not null unique,
  name text not null,
  allowed_domains text[],
  rate_tier text not null default 'standard',
  created_at timestamptz not null default now(),
  revoked_at timestamptz
);

create index api_keys_tenant_idx on api_keys (tenant_id);

grant select, insert, update on api_keys to brain_app;

alter table api_keys enable row level security;

create policy tenant_isolation on api_keys
  for all to brain_app
  using (tenant_id = public.app_tenant_id())
  with check (tenant_id = public.app_tenant_id());

create function public.api_key_lookup(p_key_hash text)
returns table (
  key_id uuid,
  tenant_id uuid,
  person_id uuid,
  person_role text,
  display_name text,
  allowed_domains text[],
  rate_tier text
)
language sql
stable
security definer
set search_path = public
as $$
  select k.id, k.tenant_id, k.person_id, p.role, p.display_name, k.allowed_domains, k.rate_tier
  from api_keys k
  join people p on p.id = k.person_id
  where k.key_hash = p_key_hash and k.revoked_at is null
$$;

revoke all on function public.api_key_lookup from public;
grant execute on function public.api_key_lookup to brain_app;

alter table audit_log add column detail jsonb not null default '{}';

alter table drift_proposals add column origin text not null default 'drift_engine'
  check (origin in ('drift_engine', 'agent', 'cold_start', 'manual'));
