create extension if not exists pg_jsonschema with schema extensions;

alter table canon_entries drop constraint canon_entries_status_check;
alter table canon_entries add constraint canon_entries_status_check
  check (status in ('draft', 'active', 'decayed', 'archived'));
alter table canon_entries add column verified_at timestamptz;

alter table canon_versions add column attributes jsonb not null default '{}';
alter table canon_versions add column status text not null default 'pending'
  check (status in ('pending', 'approved', 'rejected'));

alter table tenants add column entry_budget int not null default 150;

alter table drift_proposals add column drafted_attributes jsonb not null default '{}';
alter table drift_proposals add column domain text;
alter table drift_proposals add column pending_version_id uuid references canon_versions(id);
alter table drift_proposals add column resolved_at timestamptz;
alter table drift_proposals add column resolution_note text;
alter table drift_proposals add constraint drift_proposals_resolution_check
  check (resolution is null or resolution in ('approved', 'wrong', 'duplicate', 'not_canon_worthy', 'bad_draft', 'other'));

create table domain_schemas (
  id uuid primary key default gen_random_uuid(),
  tenant_id uuid references tenants(id),
  domain text not null,
  version int not null,
  schema jsonb not null,
  created_at timestamptz not null default now(),
  unique nulls not distinct (tenant_id, domain, version)
);

grant select on domain_schemas to brain_app;

alter table domain_schemas enable row level security;

create policy tenant_or_platform_default on domain_schemas
  for select to brain_app
  using (tenant_id is null or tenant_id = public.app_tenant_id());

insert into domain_schemas (domain, version, schema) values
  ('pricing', 1, '{"type": "object", "properties": {"product": {"type": "string"}, "plan": {"type": "string"}, "amount": {"type": "number"}, "currency": {"type": "string"}, "billing_period": {"type": "string"}, "effective_from": {"type": "string"}, "discount_ceiling_percent": {"type": "number"}, "approval_threshold": {"type": "number"}}}'),
  ('org', 1, '{"type": "object", "properties": {"unit": {"type": "string"}, "lead": {"type": "string"}, "reports_to": {"type": "string"}, "headcount": {"type": "number"}, "mandate": {"type": "string"}}}'),
  ('policy', 1, '{"type": "object", "properties": {"applies_to": {"type": "string"}, "rule": {"type": "string"}, "exceptions": {"type": "array", "items": {"type": "string"}}, "escalation_path": {"type": "string"}}}'),
  ('positioning', 1, '{"type": "object", "properties": {"audience": {"type": "string"}, "claim": {"type": "string"}, "proof_points": {"type": "array", "items": {"type": "string"}}, "banned_claims": {"type": "array", "items": {"type": "string"}}}}'),
  ('process', 1, '{"type": "object", "properties": {"trigger": {"type": "string"}, "steps": {"type": "array", "items": {"type": "string"}}, "owner_role": {"type": "string"}, "sla": {"type": "string"}}}'),
  ('decision', 1, '{"type": "object", "properties": {"question": {"type": "string"}, "outcome": {"type": "string"}, "reasoning": {"type": "string"}, "decided_at": {"type": "string"}, "revisit_condition": {"type": "string"}}}');

create function public.canon_validate_attributes() returns trigger
language plpgsql
set search_path = public, extensions
as $$
declare
  v_domain text;
  v_schema jsonb;
begin
  select ce.domain into v_domain from canon_entries ce where ce.id = new.entry_id;
  if v_domain is null then
    raise exception 'canon entry % not found', new.entry_id;
  end if;
  select ds.schema into v_schema
  from domain_schemas ds
  where ds.domain = v_domain
    and (ds.tenant_id = new.tenant_id or ds.tenant_id is null)
  order by (ds.tenant_id is not null) desc, ds.version desc
  limit 1;
  if v_schema is not null and not extensions.jsonb_matches_schema(v_schema::json, new.attributes) then
    raise exception 'attributes for domain % violate the domain schema', v_domain
      using errcode = '23514';
  end if;
  return new;
end;
$$;

create trigger canon_versions_validate_attributes
  before insert on canon_versions
  for each row execute function public.canon_validate_attributes();

create function public.role_rank(p_role text) returns int
language sql
immutable
as $$
  select case p_role
    when 'admin' then 4
    when 'owner' then 3
    when 'member' then 2
    when 'agent' then 1
    else 0
  end
$$;

create function public.canon_submit_version(
  p_entry_id uuid,
  p_created_by uuid,
  p_statement text,
  p_attributes jsonb default '{}',
  p_source_event_ids uuid[] default '{}',
  p_proposal_id uuid default null
) returns uuid
language plpgsql
set search_path = public, extensions
as $$
declare
  v_tenant uuid := public.app_tenant_id();
  v_version uuid;
  v_number int;
  v_linked int;
begin
  if v_tenant is null then
    raise exception 'no tenant context';
  end if;
  perform 1 from canon_entries ce where ce.id = p_entry_id;
  if not found then
    raise exception 'canon entry % not found', p_entry_id;
  end if;

  select coalesce(max(cv.version_number), 0) + 1 into v_number
  from canon_versions cv where cv.entry_id = p_entry_id;

  insert into canon_versions (tenant_id, entry_id, version_number, statement, created_by, attributes)
  values (v_tenant, p_entry_id, v_number, p_statement, p_created_by, p_attributes)
  returning id into v_version;

  if array_length(p_source_event_ids, 1) > 0 then
    insert into canon_provenance (tenant_id, version_id, event_id, event_occurred_at)
    select v_tenant, v_version, e.id, e.occurred_at
    from events e where e.id = any(p_source_event_ids);
    get diagnostics v_linked = row_count;
    if v_linked <> array_length(p_source_event_ids, 1) then
      raise exception 'provenance rejected, % of % source events not found',
        array_length(p_source_event_ids, 1) - v_linked, array_length(p_source_event_ids, 1);
    end if;
  end if;

  if p_proposal_id is not null then
    update drift_proposals dp set pending_version_id = v_version
    where dp.id = p_proposal_id and dp.status = 'pending';
    if not found then
      raise exception 'drift proposal % not found or not pending', p_proposal_id;
    end if;
  end if;

  insert into audit_log (tenant_id, actor_id, action, subject_type, subject_id)
  values (v_tenant, p_created_by, 'canon.version.submitted', 'canon_version', v_version);

  return v_version;
end;
$$;

create function public.canon_create_entry_draft(
  p_domain text,
  p_tier text,
  p_owner_id uuid,
  p_statement text,
  p_attributes jsonb default '{}',
  p_visibility jsonb default '{"scope": "tenant"}',
  p_verify_interval interval default interval '60 days',
  p_source_event_ids uuid[] default '{}'
) returns table (entry_id uuid, version_id uuid)
language plpgsql
set search_path = public, extensions
as $$
declare
  v_tenant uuid := public.app_tenant_id();
  v_budget int;
  v_count int;
  v_entry uuid;
  v_version uuid;
begin
  if v_tenant is null then
    raise exception 'no tenant context';
  end if;
  select t.entry_budget into v_budget from tenants t where t.id = v_tenant;
  select count(*) into v_count from canon_entries ce where ce.status <> 'archived';
  if v_count >= v_budget then
    raise exception 'entry budget of % reached, merge or archive an entry first', v_budget;
  end if;

  insert into canon_entries (tenant_id, domain, tier, owner_id, status, visibility, verify_interval)
  values (v_tenant, p_domain, p_tier, p_owner_id, 'draft', p_visibility, p_verify_interval)
  returning id into v_entry;

  v_version := public.canon_submit_version(v_entry, p_owner_id, p_statement, p_attributes, p_source_event_ids, null);
  return query select v_entry, v_version;
end;
$$;

create function public.canon_approve(
  p_version_id uuid,
  p_approver_id uuid,
  p_note text default null
) returns text
language plpgsql
set search_path = public, extensions
as $$
declare
  v_tenant uuid := public.app_tenant_id();
  v_version record;
  v_approver_role text;
  v_required_role text;
  v_required_count int;
  v_approvals int;
begin
  perform pg_advisory_xact_lock(hashtextextended(p_version_id::text, 0));

  select cv.id, cv.entry_id, cv.status, ce.tier, ce.domain
  into v_version
  from canon_versions cv
  join canon_entries ce on ce.id = cv.entry_id
  where cv.id = p_version_id;
  if not found then
    raise exception 'canon version % not found', p_version_id;
  end if;
  if v_version.status <> 'pending' then
    raise exception 'canon version % is %, only pending versions can be approved', p_version_id, v_version.status;
  end if;

  select p.role into v_approver_role from people p where p.id = p_approver_id;
  if v_approver_role is null then
    raise exception 'approver % not found', p_approver_id;
  end if;

  select ap.required_role, ap.required_approver_count
  into v_required_role, v_required_count
  from approval_policies ap
  where ap.tier = v_version.tier and (ap.domain = v_version.domain or ap.domain is null)
  order by ap.domain nulls last
  limit 1;
  if v_required_role is null then
    v_required_role := 'owner';
    v_required_count := 1;
  end if;

  if public.role_rank(v_approver_role) < public.role_rank(v_required_role) then
    raise exception 'approval of % tier % entries requires the % role', v_version.tier, v_version.domain, v_required_role;
  end if;

  insert into approvals (tenant_id, version_id, approver_id, decision, note)
  values (v_tenant, p_version_id, p_approver_id, 'approved', p_note);

  select count(distinct a.approver_id) into v_approvals
  from approvals a
  where a.version_id = p_version_id and a.decision = 'approved';

  if v_approvals < v_required_count then
    insert into audit_log (tenant_id, actor_id, action, subject_type, subject_id)
    values (v_tenant, p_approver_id, 'canon.approval.recorded', 'canon_version', p_version_id);
    return 'pending_approval';
  end if;

  update canon_versions cv set status = 'approved' where cv.id = p_version_id;
  update canon_entries ce
  set current_version_id = p_version_id, status = 'active', verified_at = now()
  where ce.id = v_version.entry_id;
  update drift_proposals dp
  set status = 'resolved', resolution = 'approved', resolved_at = now()
  where dp.pending_version_id = p_version_id and dp.status = 'pending';

  insert into audit_log (tenant_id, actor_id, action, subject_type, subject_id)
  values (v_tenant, p_approver_id, 'canon.approved', 'canon_version', p_version_id);
  return 'active';
end;
$$;

create function public.canon_reject(
  p_version_id uuid,
  p_approver_id uuid,
  p_reason text,
  p_note text default null
) returns void
language plpgsql
set search_path = public, extensions
as $$
declare
  v_tenant uuid := public.app_tenant_id();
  v_version record;
  v_approver_role text;
  v_required_role text;
begin
  if p_reason not in ('wrong', 'duplicate', 'not_canon_worthy', 'bad_draft', 'other') then
    raise exception 'rejection reason must be one of wrong, duplicate, not_canon_worthy, bad_draft, other';
  end if;

  perform pg_advisory_xact_lock(hashtextextended(p_version_id::text, 0));

  select cv.id, cv.entry_id, cv.status, ce.tier, ce.domain
  into v_version
  from canon_versions cv
  join canon_entries ce on ce.id = cv.entry_id
  where cv.id = p_version_id;
  if not found then
    raise exception 'canon version % not found', p_version_id;
  end if;
  if v_version.status <> 'pending' then
    raise exception 'canon version % is %, only pending versions can be rejected', p_version_id, v_version.status;
  end if;

  select p.role into v_approver_role from people p where p.id = p_approver_id;
  if v_approver_role is null then
    raise exception 'approver % not found', p_approver_id;
  end if;
  select ap.required_role into v_required_role
  from approval_policies ap
  where ap.tier = v_version.tier and (ap.domain = v_version.domain or ap.domain is null)
  order by ap.domain nulls last
  limit 1;
  if v_required_role is null then
    v_required_role := 'owner';
  end if;
  if public.role_rank(v_approver_role) < public.role_rank(v_required_role) then
    raise exception 'rejection of % tier entries requires the % role', v_version.tier, v_required_role;
  end if;

  insert into approvals (tenant_id, version_id, approver_id, decision, note)
  values (v_tenant, p_version_id, p_approver_id, 'rejected', p_note);

  update canon_versions cv set status = 'rejected' where cv.id = p_version_id;
  update drift_proposals dp
  set status = 'resolved', resolution = p_reason, resolution_note = p_note, resolved_at = now()
  where dp.pending_version_id = p_version_id and dp.status = 'pending';

  insert into audit_log (tenant_id, actor_id, action, subject_type, subject_id)
  values (v_tenant, p_approver_id, 'canon.rejected', 'canon_version', p_version_id);
end;
$$;

create function public.canon_decay_sweep() returns int
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

  with decayed as (
    update canon_entries ce set status = 'decayed'
    where ce.status = 'active'
      and ce.verified_at is not null
      and ce.verified_at + ce.verify_interval < now()
    returning ce.id, ce.tenant_id, ce.domain, ce.owner_id, ce.current_version_id
  ),
  proposals as (
    insert into drift_proposals (tenant_id, entry_id, kind, drafted_statement, confidence, routed_to, domain)
    select d.tenant_id, d.id, 'decay', coalesce(cv.statement, ''), 1.0, d.owner_id, d.domain
    from decayed d
    left join canon_versions cv on cv.id = d.current_version_id
    returning id
  ),
  audits as (
    insert into audit_log (tenant_id, actor_id, action, subject_type, subject_id)
    select d.tenant_id, null, 'canon.decayed', 'canon_entry', d.id from decayed d
    returning id
  )
  select count(*) into v_count from decayed;

  return v_count;
end;
$$;

create function public.canon_health() returns table (
  domain text,
  score numeric,
  verified_share numeric,
  median_resolution_hours numeric,
  open_contradictions bigint,
  gap_acceptance numeric
)
language sql
stable
set search_path = public, extensions
as $$
  with entries as (
    select ce.domain, ce.status, ce.verified_at, ce.verify_interval
    from canon_entries ce where ce.status <> 'archived'
  ),
  verified as (
    select e.domain,
           count(*) filter (
             where e.status = 'active' and e.verified_at is not null
               and e.verified_at + e.verify_interval >= now()
           )::numeric / nullif(count(*), 0) as share
    from entries e group by e.domain
  ),
  proposal_domains as (
    select coalesce(ce.domain, dp.domain) as domain, dp.kind, dp.status,
           dp.resolution, dp.created_at, dp.resolved_at
    from drift_proposals dp
    left join canon_entries ce on ce.id = dp.entry_id
  ),
  resolution as (
    select pd.domain,
           percentile_cont(0.5) within group (
             order by extract(epoch from pd.resolved_at - pd.created_at)
           ) as median_secs
    from proposal_domains pd where pd.resolved_at is not null
    group by pd.domain
  ),
  contradictions as (
    select pd.domain, count(*) as open
    from proposal_domains pd
    where pd.kind = 'contradiction' and pd.status = 'pending'
    group by pd.domain
  ),
  gaps as (
    select pd.domain,
           count(*) filter (where pd.resolution = 'approved')::numeric
             / nullif(count(*) filter (where pd.status = 'resolved'), 0) as acceptance
    from proposal_domains pd where pd.kind = 'gap'
    group by pd.domain
  )
  select d.domain,
         round((
           coalesce(v.share, 0) * 40
           + coalesce(least(1, 604800 / nullif(r.median_secs, 0)), 1) * 25
           + greatest(0, 1 - coalesce(c.open, 0) / 5.0) * 20
           + coalesce(g.acceptance, 1) * 15
         )::numeric, 1) as score,
         round(coalesce(v.share, 0), 3) as verified_share,
         round((r.median_secs / 3600.0)::numeric, 1) as median_resolution_hours,
         coalesce(c.open, 0) as open_contradictions,
         round(coalesce(g.acceptance, 1), 3) as gap_acceptance
  from (select distinct e.domain from entries e) d
  left join verified v on v.domain = d.domain
  left join resolution r on r.domain = d.domain
  left join contradictions c on c.domain = d.domain
  left join gaps g on g.domain = d.domain
$$;

grant update (status) on canon_versions to brain_app;
grant execute on function public.role_rank to brain_app;
grant execute on function public.canon_submit_version to brain_app;
grant execute on function public.canon_create_entry_draft to brain_app;
grant execute on function public.canon_approve to brain_app;
grant execute on function public.canon_reject to brain_app;
grant execute on function public.canon_decay_sweep to brain_app;
grant execute on function public.canon_health to brain_app;
