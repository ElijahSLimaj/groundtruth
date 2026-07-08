begin;
create extension if not exists pgtap with schema extensions;
set local search_path = public, extensions;

select plan(16);

insert into tenants (id, name, tier) values
  ('00000000-0000-0000-0000-0000000000aa', 'Merge Tenant', 'growth');

insert into people (id, tenant_id, email, display_name, role) values
  ('00000000-0000-0000-0001-0000000000aa', '00000000-0000-0000-0000-0000000000aa', 'older@m.test', 'Older Owner', 'owner'),
  ('00000000-0000-0000-0002-0000000000aa', '00000000-0000-0000-0000-0000000000aa', 'newer@m.test', 'Newer Owner', 'owner'),
  ('00000000-0000-0000-0003-0000000000aa', '00000000-0000-0000-0000-0000000000aa', 'member@m.test', 'Member', 'member');

insert into canon_entries (id, tenant_id, domain, tier, owner_id, status, visibility, verify_interval, created_at) values
  ('00000000-0000-0003-0001-0000000000aa', '00000000-0000-0000-0000-0000000000aa', 'pricing', 'operational', '00000000-0000-0000-0001-0000000000aa', 'active', '{"scope": "tenant"}', interval '60 days', '2026-01-01'),
  ('00000000-0000-0003-0002-0000000000aa', '00000000-0000-0000-0000-0000000000aa', 'pricing', 'operational', '00000000-0000-0000-0002-0000000000aa', 'active', '{"scope": "tenant"}', interval '60 days', '2026-02-01'),
  ('00000000-0000-0003-0003-0000000000aa', '00000000-0000-0000-0000-0000000000aa', 'pricing', 'operational', '00000000-0000-0000-0001-0000000000aa', 'active', '{"scope": "tenant"}', interval '60 days', '2026-03-01');

insert into canon_versions (id, tenant_id, entry_id, version_number, statement, created_by, status) values
  ('00000000-0000-0004-0001-0000000000aa', '00000000-0000-0000-0000-0000000000aa', '00000000-0000-0003-0001-0000000000aa', 1, 'Growth plan costs 1499 per month', '00000000-0000-0000-0001-0000000000aa', 'approved'),
  ('00000000-0000-0004-0002-0000000000aa', '00000000-0000-0000-0000-0000000000aa', '00000000-0000-0003-0002-0000000000aa', 1, 'The growth plan is 1499 monthly', '00000000-0000-0000-0002-0000000000aa', 'approved'),
  ('00000000-0000-0004-0003-0000000000aa', '00000000-0000-0000-0000-0000000000aa', '00000000-0000-0003-0003-0000000000aa', 1, 'Enterprise contracts renew annually', '00000000-0000-0000-0001-0000000000aa', 'approved');

update canon_entries set current_version_id = '00000000-0000-0004-0001-0000000000aa' where id = '00000000-0000-0003-0001-0000000000aa';
update canon_entries set current_version_id = '00000000-0000-0004-0002-0000000000aa' where id = '00000000-0000-0003-0002-0000000000aa';
update canon_entries set current_version_id = '00000000-0000-0004-0003-0000000000aa' where id = '00000000-0000-0003-0003-0000000000aa';

insert into canon_statement_embeddings (version_id, embedding_model, tenant_id, embedding) values
  ('00000000-0000-0004-0001-0000000000aa', 'test-model', '00000000-0000-0000-0000-0000000000aa', array_fill(0.5, array[1536])::vector),
  ('00000000-0000-0004-0002-0000000000aa', 'test-model', '00000000-0000-0000-0000-0000000000aa', array_fill(0.5, array[1536])::vector),
  ('00000000-0000-0004-0003-0000000000aa', 'test-model', '00000000-0000-0000-0000-0000000000aa',
   (select array_agg(case when i <= 768 then 1.0 else 0.0 end)::vector from generate_series(1, 1536) i));

set local role brain_app;
select set_config('app.tenant_id', '00000000-0000-0000-0000-0000000000aa', true);

select is(
  public.canon_merge_detect('test-model', 0.9),
  1,
  'detect proposes exactly the near-duplicate pair'
);

select is(
  (select entry_id from drift_proposals where kind = 'merge'),
  '00000000-0000-0003-0002-0000000000aa'::uuid,
  'the newer entry is the merge source'
);

select is(
  (select merge_into_entry_id from drift_proposals where kind = 'merge'),
  '00000000-0000-0003-0001-0000000000aa'::uuid,
  'the older entry survives the merge'
);

select is(
  (select routed_to from drift_proposals where kind = 'merge'),
  '00000000-0000-0000-0001-0000000000aa'::uuid,
  'the proposal routes to the surviving owner'
);

select is(
  (select status from drift_proposals where kind = 'merge'),
  'pending',
  'the proposal lands pending'
);

select is(
  public.canon_merge_detect('test-model', 0.9),
  0,
  'an open proposal suppresses re-detection'
);

select throws_ok(
  $$select public.canon_merge_apply(
      (select id from drift_proposals where kind = 'merge'),
      '00000000-0000-0000-0003-0000000000aa')$$,
  'merging entries requires the owner role',
  'members cannot apply merges'
);

select lives_ok(
  $$select public.canon_merge_apply(
      (select id from drift_proposals where kind = 'merge'),
      '00000000-0000-0000-0001-0000000000aa', 'same fact')$$,
  'owners apply merges'
);

select is(
  (select status from canon_entries where id = '00000000-0000-0003-0002-0000000000aa'),
  'archived',
  'the merged source entry is archived'
);

select is(
  (select status from canon_entries where id = '00000000-0000-0003-0001-0000000000aa'),
  'active',
  'the surviving entry stays active'
);

select is(
  (select count(*) from canon_relations
   where from_entry = '00000000-0000-0003-0001-0000000000aa'
     and to_entry = '00000000-0000-0003-0002-0000000000aa'
     and relation = 'supersedes'),
  1::bigint,
  'the survivor supersedes the archived entry'
);

select is(
  (select status from drift_proposals where kind = 'merge'),
  'resolved',
  'the merge proposal resolves'
);

select is(
  (select resolution from drift_proposals where kind = 'merge'),
  'approved',
  'the resolution records the approval'
);

select is(
  (select count(*) from audit_log where action = 'canon.merged'),
  1::bigint,
  'merges are audited'
);

select is(
  public.canon_merge_detect('test-model', 0.9),
  0,
  'archived entries never re-enter detection'
);

select set_config('app.tenant_id', null, true);
select throws_ok(
  $$select public.canon_merge_detect('test-model', 0.9)$$,
  'no tenant context',
  'detection requires tenant context'
);

select * from finish();
rollback;
