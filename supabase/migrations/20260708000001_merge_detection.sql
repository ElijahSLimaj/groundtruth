alter table drift_proposals drop constraint drift_proposals_kind_check;
alter table drift_proposals add constraint drift_proposals_kind_check
  check (kind in ('contradiction', 'extension', 'gap', 'decay', 'merge'));

alter table drift_proposals add column merge_into_entry_id uuid references canon_entries(id);
alter table drift_proposals add constraint drift_proposals_merge_pair_check
  check (
    kind <> 'merge'
    or (entry_id is not null and merge_into_entry_id is not null and entry_id <> merge_into_entry_id)
  );

create unique index drift_proposals_open_merge_pair_idx
  on drift_proposals (
    tenant_id,
    least(entry_id, merge_into_entry_id),
    greatest(entry_id, merge_into_entry_id)
  )
  where kind = 'merge' and status in ('pending', 'queued');

create function public.canon_merge_detect(p_model text, p_threshold numeric) returns int
language plpgsql
set search_path = public, extensions
as $$
declare
  v_tenant uuid := public.app_tenant_id();
  v_count int;
begin
  if v_tenant is null then
    raise exception 'no tenant context';
  end if;

  with candidates as (
    select ce.id as entry_id, ce.domain, ce.owner_id, ce.created_at,
           cv.id as version_id, cv.statement, cse.embedding
    from canon_entries ce
    join canon_versions cv on cv.id = ce.current_version_id
    join canon_statement_embeddings cse
      on cse.version_id = cv.id and cse.embedding_model = p_model
    where ce.status = 'active'
  ),
  pairs as (
    select newer.entry_id as source_entry, older.entry_id as target_entry,
           newer.version_id as source_version, older.version_id as target_version,
           newer.owner_id as source_owner, older.owner_id as target_owner,
           older.domain, older.statement as target_statement,
           1 - (newer.embedding <=> older.embedding) as similarity
    from candidates newer
    join candidates older
      on older.domain = newer.domain
     and (older.created_at, older.entry_id) < (newer.created_at, newer.entry_id)
    where 1 - (newer.embedding <=> older.embedding) >= p_threshold
  ),
  fresh as (
    select p.* from pairs p
    where not exists (
      select 1 from drift_proposals dp
      where dp.kind = 'merge'
        and least(dp.entry_id, dp.merge_into_entry_id)
              = least(p.source_entry, p.target_entry)
        and greatest(dp.entry_id, dp.merge_into_entry_id)
              = greatest(p.source_entry, p.target_entry)
        and (dp.status in ('pending', 'queued')
             or (dp.drafted_attributes ->> 'source_version_id' = p.source_version::text
                 and dp.drafted_attributes ->> 'target_version_id' = p.target_version::text))
    )
  ),
  proposals as (
    insert into drift_proposals
      (tenant_id, entry_id, merge_into_entry_id, kind, drafted_statement,
       drafted_attributes, confidence, routed_to, domain)
    select v_tenant, f.source_entry, f.target_entry, 'merge', f.target_statement,
           jsonb_build_object(
             'source_version_id', f.source_version,
             'target_version_id', f.target_version,
             'similarity', round(f.similarity::numeric, 4)
           ),
           f.similarity,
           case when f.source_owner = f.target_owner then f.source_owner
                else f.target_owner end,
           f.domain
    from fresh f
    returning id
  )
  select count(*) into v_count from proposals;

  return v_count;
end;
$$;

create function public.canon_merge_apply(
  p_proposal_id uuid,
  p_reviewer_id uuid,
  p_note text default null
) returns void
language plpgsql
set search_path = public, extensions
as $$
declare
  v_tenant uuid := public.app_tenant_id();
  v_proposal record;
  v_reviewer_role text;
begin
  if v_tenant is null then
    raise exception 'no tenant context';
  end if;

  perform pg_advisory_xact_lock(hashtextextended(p_proposal_id::text, 0));

  select dp.id, dp.kind, dp.status, dp.entry_id, dp.merge_into_entry_id
  into v_proposal
  from drift_proposals dp
  where dp.id = p_proposal_id;
  if not found then
    raise exception 'proposal % not found', p_proposal_id;
  end if;
  if v_proposal.kind <> 'merge' then
    raise exception 'proposal % is a % proposal, not a merge', p_proposal_id, v_proposal.kind;
  end if;
  if v_proposal.status not in ('pending', 'queued') then
    raise exception 'proposal % is already resolved', p_proposal_id;
  end if;

  select p.role into v_reviewer_role from people p where p.id = p_reviewer_id;
  if v_reviewer_role is null then
    raise exception 'reviewer % not found', p_reviewer_id;
  end if;
  if public.role_rank(v_reviewer_role) < public.role_rank('owner') then
    raise exception 'merging entries requires the owner role';
  end if;

  update canon_entries set status = 'archived'
  where id = v_proposal.entry_id and status <> 'archived';

  insert into canon_relations (tenant_id, from_entry, to_entry, relation)
  values (v_tenant, v_proposal.merge_into_entry_id, v_proposal.entry_id, 'supersedes')
  on conflict do nothing;

  update drift_proposals
  set status = 'resolved', resolution = 'approved', resolution_note = p_note, resolved_at = now()
  where id = p_proposal_id;

  insert into audit_log (tenant_id, actor_id, action, subject_type, subject_id, detail)
  values (v_tenant, p_reviewer_id, 'canon.merged', 'canon_entry', v_proposal.entry_id,
          jsonb_build_object('into_entry_id', v_proposal.merge_into_entry_id,
                             'proposal_id', p_proposal_id));
end;
$$;

grant execute on function public.canon_merge_detect to brain_app;
grant execute on function public.canon_merge_apply to brain_app;
