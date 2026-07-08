alter table events add column tombstoned boolean not null default false;
alter table canon_provenance add column erased boolean not null default false;

create table erasure_requests (
  id uuid primary key default gen_random_uuid(),
  tenant_id uuid not null references tenants(id),
  person_id uuid not null references people(id),
  requested_by uuid not null references people(id),
  reason text not null,
  status text not null default 'pending'
    check (status in ('pending', 'verified', 'completed', 'rejected')),
  verified_by uuid references people(id),
  verified_at timestamptz,
  completed_at timestamptz,
  created_at timestamptz not null default now()
);

grant select, insert, update on erasure_requests to brain_app;
grant update (tombstoned) on events to brain_app;
grant update (tombstoned) on event_chunks to brain_app;
grant update (erased) on canon_provenance to brain_app;

alter table erasure_requests enable row level security;

create policy tenant_isolation on erasure_requests
  for all to brain_app
  using (tenant_id = public.app_tenant_id())
  with check (tenant_id = public.app_tenant_id());

create function public.event_tombstone(p_event_id uuid, p_actor_id uuid default null)
returns int
language plpgsql
set search_path = public, extensions
as $$
declare
  v_tenant uuid := public.app_tenant_id();
  v_chunks int;
begin
  if v_tenant is null then
    raise exception 'no tenant context';
  end if;

  update events set tombstoned = true
  where id = p_event_id and not tombstoned;
  if not found then
    raise exception 'event % not found or already tombstoned', p_event_id;
  end if;

  update event_chunks set tombstoned = true
  where event_id = p_event_id and not tombstoned;
  get diagnostics v_chunks = row_count;

  insert into audit_log (tenant_id, actor_id, action, subject_type, subject_id, detail)
  values (v_tenant, p_actor_id, 'event.tombstoned', 'event', p_event_id,
          jsonb_build_object('chunks_tombstoned', v_chunks));

  return v_chunks;
end;
$$;

create function public.erasure_execute(p_request_id uuid, p_executor_id uuid)
returns table (payload_ref text)
language plpgsql
set search_path = public, extensions
as $$
declare
  v_tenant uuid := public.app_tenant_id();
  v_request record;
  v_role text;
  v_events int;
  v_chunks int;
begin
  if v_tenant is null then
    raise exception 'no tenant context';
  end if;

  perform pg_advisory_xact_lock(hashtextextended(p_request_id::text, 0));

  select er.id, er.person_id, er.status
  into v_request
  from erasure_requests er
  where er.id = p_request_id;
  if not found then
    raise exception 'erasure request % not found', p_request_id;
  end if;
  if v_request.status <> 'verified' then
    raise exception 'erasure request % is %, only verified requests can be executed',
      p_request_id, v_request.status;
  end if;

  select p.role into v_role from people p where p.id = p_executor_id;
  if coalesce(public.role_rank(v_role), 0) < public.role_rank('admin') then
    raise exception 'executing an erasure requires the admin role';
  end if;

  update events set tombstoned = true
  where author_id = v_request.person_id and not tombstoned;
  get diagnostics v_events = row_count;

  delete from event_chunks ec
  using events e
  where e.id = ec.event_id and e.author_id = v_request.person_id;
  get diagnostics v_chunks = row_count;

  update canon_provenance cp set erased = true
  from events e
  where e.id = cp.event_id
    and e.author_id = v_request.person_id
    and not cp.erased;

  update api_keys set revoked_at = now()
  where person_id = v_request.person_id and revoked_at is null;

  update people
  set email = 'erased+' || v_request.person_id || '@erased.invalid',
      display_name = 'Erased person'
  where id = v_request.person_id;

  update erasure_requests
  set status = 'completed', completed_at = now()
  where id = p_request_id;

  insert into audit_log (tenant_id, actor_id, action, subject_type, subject_id, detail)
  values (v_tenant, p_executor_id, 'person.erased', 'person', v_request.person_id,
          jsonb_build_object('request_id', p_request_id,
                             'events_tombstoned', v_events,
                             'chunks_deleted', v_chunks));

  return query
    select distinct e.payload_ref from events e
    where e.author_id = v_request.person_id
      and not exists (
        select 1 from events live
        where live.payload_ref = e.payload_ref and not live.tombstoned
      );
end;
$$;

grant execute on function public.event_tombstone to brain_app;
grant execute on function public.erasure_execute to brain_app;
