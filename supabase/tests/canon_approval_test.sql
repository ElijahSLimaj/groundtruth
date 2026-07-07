begin;
create extension if not exists pgtap with schema extensions;
set local search_path = public, extensions;

select plan(27);

insert into tenants (id, name, tier) values
  ('00000000-0000-0000-0000-00000000000a', 'Tenant A', 'growth');

insert into people (id, tenant_id, email, display_name, role) values
  ('00000000-0000-0000-0001-00000000000a', '00000000-0000-0000-0000-00000000000a', 'admin1@a.test', 'Admin One', 'admin'),
  ('00000000-0000-0000-0002-00000000000a', '00000000-0000-0000-0000-00000000000a', 'admin2@a.test', 'Admin Two', 'admin'),
  ('00000000-0000-0000-0003-00000000000a', '00000000-0000-0000-0000-00000000000a', 'owner@a.test', 'Owner', 'owner'),
  ('00000000-0000-0000-0004-00000000000a', '00000000-0000-0000-0000-00000000000a', 'member@a.test', 'Member', 'member');

insert into connectors (id, tenant_id, source_type, status, config) values
  ('00000000-0000-0001-0000-00000000000a', '00000000-0000-0000-0000-00000000000a', 'slack', 'live', '{}');

insert into events (id, tenant_id, connector_id, source_type, external_id, occurred_at, acl, payload_ref) values
  ('00000000-0000-0002-0001-00000000000a', '00000000-0000-0000-0000-00000000000a', '00000000-0000-0001-0000-00000000000a', 'slack', 'ev-1', now(), '{"scope": "tenant"}', 'payloads/a/1'),
  ('00000000-0000-0002-0002-00000000000a', '00000000-0000-0000-0000-00000000000a', '00000000-0000-0001-0000-00000000000a', 'slack', 'ev-2', now(), '{"scope": "tenant"}', 'payloads/a/2');

insert into approval_policies (tenant_id, tier, domain, required_role, required_approver_count) values
  ('00000000-0000-0000-0000-00000000000a', 'operational', null, 'owner', 1),
  ('00000000-0000-0000-0000-00000000000a', 'bedrock', null, 'admin', 2);

set local role brain_app;
select set_config('app.tenant_id', '00000000-0000-0000-0000-00000000000a', true);

create temp table draft1 as
select * from public.canon_create_entry_draft(
  'pricing', 'operational', '00000000-0000-0000-0003-00000000000a',
  'Growth plan is 1499 per month',
  '{"plan": "growth", "amount": 1499, "currency": "USD"}',
  '{"scope": "tenant"}', interval '60 days',
  array['00000000-0000-0002-0001-00000000000a', '00000000-0000-0002-0002-00000000000a']::uuid[]
);

select is(
  (select ce.status from canon_entries ce join draft1 d on ce.id = d.entry_id),
  'draft',
  'a new entry is born as a draft'
);

select is(
  (select cv.status from canon_versions cv join draft1 d on cv.id = d.version_id),
  'pending',
  'its first version starts pending'
);

select is(
  (select count(*) from canon_provenance cp join draft1 d on cp.version_id = d.version_id),
  2::bigint,
  'provenance links every source event'
);

select throws_ok(
  $$select public.canon_create_entry_draft('pricing', 'operational', '00000000-0000-0000-0003-00000000000a', 'Bad attributes', '{"amount": "not a number"}')$$,
  '23514',
  null,
  'attributes violating the domain schema reject the write'
);

select throws_ok(
  $$select public.canon_submit_version('00000000-0000-0000-0099-00000000000a', '00000000-0000-0000-0003-00000000000a', 'ghost entry', '{}', array['00000000-0000-0002-0099-00000000000a']::uuid[])$$,
  null,
  null,
  'submitting against a missing entry fails'
);

select throws_ok(
  (select format($$select public.canon_approve('%s', '00000000-0000-0000-0004-00000000000a')$$, d.version_id) from draft1 d),
  null,
  null,
  'a member cannot approve an operational entry gated on owner'
);

select is(
  (select public.canon_approve(d.version_id, '00000000-0000-0000-0003-00000000000a', 'looks right') from draft1 d),
  'active',
  'owner approval activates a single approver entry'
);

select is(
  (select ce.status || ':' || (ce.current_version_id = d.version_id)::text || ':' || (ce.verified_at is not null)::text
   from canon_entries ce join draft1 d on ce.id = d.entry_id),
  'active:true:true',
  'approval flips the pointer, activates, and stamps verified_at atomically'
);

select ok(
  exists(select 1 from audit_log al join draft1 d on al.subject_id = d.version_id
         where al.action = 'canon.approved'),
  'approval writes the audit entry'
);

select throws_ok(
  (select format($$select public.canon_approve('%s', '00000000-0000-0000-0003-00000000000a')$$, d.version_id) from draft1 d),
  null,
  null,
  'an approved version cannot be approved again'
);

create temp table draft2 as
select * from public.canon_create_entry_draft(
  'positioning', 'bedrock', '00000000-0000-0000-0001-00000000000a',
  'We sell provable trust, not AI magic',
  '{"audience": "founders and COOs", "claim": "provable trust"}'
);

select throws_ok(
  (select format($$select public.canon_approve('%s', '00000000-0000-0000-0003-00000000000a')$$, d.version_id) from draft2 d),
  null,
  null,
  'an owner cannot approve a bedrock entry gated on admin'
);

select is(
  (select public.canon_approve(d.version_id, '00000000-0000-0000-0001-00000000000a') from draft2 d),
  'pending_approval',
  'first of two required approvals holds the version pending'
);

select is(
  (select ce.status from canon_entries ce join draft2 d on ce.id = d.entry_id),
  'draft',
  'the entry stays draft until the approval count is met'
);

select is(
  (select public.canon_approve(d.version_id, '00000000-0000-0000-0001-00000000000a', 'again') from draft2 d),
  'pending_approval',
  'the same approver approving twice counts once'
);

select is(
  (select public.canon_approve(d.version_id, '00000000-0000-0000-0002-00000000000a') from draft2 d),
  'active',
  'the second distinct admin approval activates the bedrock entry'
);

insert into drift_proposals (id, tenant_id, entry_id, kind, drafted_statement, confidence, routed_to)
select '00000000-0000-0009-0001-00000000000a', '00000000-0000-0000-0000-00000000000a', d.entry_id,
       'contradiction', 'Growth plan pricing changed to 1799 per month', 0.9, '00000000-0000-0000-0003-00000000000a'
from draft1 d;

create temp table v2 as
select public.canon_submit_version(
  d.entry_id, '00000000-0000-0000-0003-00000000000a',
  'Growth plan is 1799 per month from August',
  '{"plan": "growth", "amount": 1799, "currency": "USD"}',
  array['00000000-0000-0002-0002-00000000000a']::uuid[],
  '00000000-0000-0009-0001-00000000000a'
) as version_id
from draft1 d;

select is(
  (select dp.pending_version_id from drift_proposals dp where dp.id = '00000000-0000-0009-0001-00000000000a'),
  (select v.version_id from v2 v),
  'submitting against a proposal links the pending version'
);

select is(
  (select public.canon_approve(v.version_id, '00000000-0000-0000-0003-00000000000a') from v2 v),
  'active',
  'approving the second version succeeds'
);

select is(
  (select dp.status || ':' || dp.resolution || ':' || (dp.resolved_at is not null)::text
   from drift_proposals dp where dp.id = '00000000-0000-0009-0001-00000000000a'),
  'resolved:approved:true',
  'approval resolves the linked drift proposal'
);

select is(
  (select ce.current_version_id from canon_entries ce join draft1 d on ce.id = d.entry_id),
  (select v.version_id from v2 v),
  'the pointer moved to version two'
);

select is(
  (select cv.version_number from canon_versions cv join v2 v on cv.id = v.version_id),
  2,
  'version numbers increment per entry'
);

insert into drift_proposals (id, tenant_id, entry_id, kind, drafted_statement, confidence, routed_to)
select '00000000-0000-0009-0002-00000000000a', '00000000-0000-0000-0000-00000000000a', d.entry_id,
       'contradiction', 'Growth plan is free now', 0.3, '00000000-0000-0000-0003-00000000000a'
from draft1 d;

create temp table v3 as
select public.canon_submit_version(
  d.entry_id, '00000000-0000-0000-0003-00000000000a',
  'Growth plan is free now', '{}', '{}',
  '00000000-0000-0009-0002-00000000000a'
) as version_id
from draft1 d;

select lives_ok(
  (select format($$select public.canon_reject('%s', '00000000-0000-0000-0003-00000000000a', 'wrong', 'that message was a joke')$$, v.version_id) from v3 v),
  'rejection with a taxonomy reason succeeds'
);

select is(
  (select dp.status || ':' || dp.resolution || ':' || dp.resolution_note
   from drift_proposals dp where dp.id = '00000000-0000-0009-0002-00000000000a'),
  'resolved:wrong:that message was a joke',
  'rejection resolves the proposal with the structured reason'
);

select is(
  (select ce.current_version_id from canon_entries ce join draft1 d on ce.id = d.entry_id),
  (select v.version_id from v2 v),
  'rejection leaves the previous version serving'
);

reset role;
update tenants set entry_budget = (select count(*) from canon_entries where tenant_id = '00000000-0000-0000-0000-00000000000a' and status <> 'archived')
  where id = '00000000-0000-0000-0000-00000000000a';
update canon_entries set verified_at = now() - interval '200 days'
  where id = (select d.entry_id from draft2 d);
set local role brain_app;
select set_config('app.tenant_id', '00000000-0000-0000-0000-00000000000a', true);

select throws_ok(
  $$select public.canon_create_entry_draft('org', 'operational', '00000000-0000-0000-0003-00000000000a', 'one entry too many')$$,
  null,
  null,
  'the entry budget blocks new drafts when reached'
);

select is(
  (select public.canon_decay_sweep()),
  1,
  'the decay sweep flips the overdue entry'
);

select is(
  (select ce.status || ':' || exists(
     select 1 from drift_proposals dp
     where dp.entry_id = ce.id and dp.kind = 'decay' and dp.status = 'pending'
       and dp.routed_to = ce.owner_id)::text
   from canon_entries ce join draft2 d on ce.id = d.entry_id),
  'decayed:true',
  'decay flags the entry and routes a proposal to its owner'
);

select ok(
  (select bool_and(score between 0 and 100) from public.canon_health()),
  'canon health scores stay within bounds'
);

select * from finish();
rollback;
