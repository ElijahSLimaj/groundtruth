create table web_sessions (
  id uuid primary key default gen_random_uuid(),
  tenant_id uuid not null references tenants(id),
  person_id uuid not null references people(id),
  token_hash text not null unique,
  created_at timestamptz not null default now(),
  expires_at timestamptz not null,
  last_seen_at timestamptz not null default now()
);

create index web_sessions_expiry_idx on web_sessions (expires_at);

grant select, insert, update, delete on web_sessions to brain_app;

alter table web_sessions enable row level security;

create policy tenant_isolation on web_sessions
  for all to brain_app
  using (tenant_id = public.app_tenant_id())
  with check (tenant_id = public.app_tenant_id());

create function public.web_person_by_email(p_email text)
returns table (tenant_id uuid, person_id uuid, role text, display_name text)
language sql
stable
security definer
set search_path = public, extensions
as $$
  select p.tenant_id, p.id, p.role, p.display_name
  from people p
  join tenants t on t.id = p.tenant_id
  where lower(p.email) = lower(p_email)
  order by t.created_at, t.id;
$$;

create function public.web_session_lookup(p_token_hash text)
returns table (
  tenant_id uuid,
  person_id uuid,
  role text,
  display_name text,
  email text
)
language plpgsql
security definer
set search_path = public, extensions
as $$
begin
  return query
  update web_sessions s
  set last_seen_at = now()
  from people p
  where p.id = s.person_id
    and s.token_hash = p_token_hash
    and s.expires_at > now()
  returning s.tenant_id, s.person_id, p.role, p.display_name, p.email;
end;
$$;

create function public.web_session_destroy(p_token_hash text)
returns int
language plpgsql
security definer
set search_path = public, extensions
as $$
declare
  v_count int;
begin
  delete from web_sessions where token_hash = p_token_hash;
  get diagnostics v_count = row_count;
  delete from web_sessions where expires_at < now() - interval '7 days';
  return v_count;
end;
$$;

revoke execute on function public.web_person_by_email from public;
revoke execute on function public.web_session_lookup from public;
revoke execute on function public.web_session_destroy from public;
grant execute on function public.web_person_by_email to brain_app;
grant execute on function public.web_session_lookup to brain_app;
grant execute on function public.web_session_destroy to brain_app;
