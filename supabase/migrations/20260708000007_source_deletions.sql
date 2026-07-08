create function public.event_tombstone_by_external(p_connector_id uuid, p_external_id text)
returns int
language plpgsql
security definer
set search_path = public, extensions
as $$
declare
  v_events int;
begin
  with dead as (
    update events set tombstoned = true
    where connector_id = p_connector_id
      and external_id = p_external_id
      and not tombstoned
    returning id, tenant_id
  ),
  chunks as (
    update event_chunks ec set tombstoned = true
    from dead d
    where ec.event_id = d.id and not ec.tombstoned
    returning ec.id
  ),
  audits as (
    insert into audit_log (tenant_id, actor_id, action, subject_type, subject_id, detail)
    select d.tenant_id, null, 'event.tombstoned', 'event', d.id,
           jsonb_build_object('reason', 'source_deletion')
    from dead d
    returning id
  )
  select count(*) into v_events from dead;

  return v_events;
end;
$$;

revoke execute on function public.event_tombstone_by_external from public;
grant execute on function public.event_tombstone_by_external to brain_worker;
grant execute on function public.event_tombstone_by_external to brain_app;
