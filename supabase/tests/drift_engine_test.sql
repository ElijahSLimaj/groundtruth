begin;
create extension if not exists pgtap with schema extensions;
set local search_path = public, extensions;

select plan(8);

insert into tenants (id, name, tier) values
  ('00000000-0000-0000-0000-00000000000a', 'Tenant A', 'growth'),
  ('00000000-0000-0000-0000-00000000000b', 'Tenant B', 'growth');

insert into people (id, tenant_id, email, display_name, role) values
  ('00000000-0000-0000-0001-00000000000a', '00000000-0000-0000-0000-00000000000a', 'owner@a.test', 'Owner A', 'owner');

insert into canon_entries (id, tenant_id, domain, tier, owner_id, status, visibility, verify_interval) values
  ('00000000-0000-0003-0000-00000000000a', '00000000-0000-0000-0000-00000000000a', 'pricing', 'operational', '00000000-0000-0000-0001-00000000000a', 'active', '{"scope": "tenant"}', interval '60 days');

insert into canon_versions (id, tenant_id, entry_id, version_number, statement, created_by, status) values
  ('00000000-0000-0004-0000-00000000000a', '00000000-0000-0000-0000-00000000000a', '00000000-0000-0003-0000-00000000000a', 1, 'Growth is 1499', '00000000-0000-0000-0001-00000000000a', 'approved');

set local role brain_embedder;

select is(
  (select count(*) from canon_versions cv join canon_entries ce on ce.id = cv.entry_id
   where ce.status = 'active' and cv.id = '00000000-0000-0004-0000-00000000000a'),
  1::bigint,
  'embedder reads canon versions for statement embedding'
);

select lives_ok(
  $$insert into canon_statement_embeddings (version_id, embedding_model, tenant_id, embedding)
    values ('00000000-0000-0004-0000-00000000000a', 'test-model', '00000000-0000-0000-0000-00000000000a', array_fill(0.5, array[1536])::vector)$$,
  'embedder writes statement embeddings'
);

reset role;
set local role brain_app;
select set_config('app.tenant_id', '00000000-0000-0000-0000-00000000000a', true);

select is(
  (select count(*) from canon_statement_embeddings),
  1::bigint,
  'app role reads statement embeddings in its tenant'
);

select throws_ok(
  $$insert into canon_statement_embeddings (version_id, embedding_model, tenant_id, embedding)
    values ('00000000-0000-0004-0000-00000000000a', 'other-model', '00000000-0000-0000-0000-00000000000a', array_fill(0.5, array[1536])::vector)$$,
  '42501',
  null,
  'app role cannot write statement embeddings'
);

select lives_ok(
  $$insert into drift_state (tenant_id) values ('00000000-0000-0000-0000-00000000000a')$$,
  'app role manages its drift watermark'
);

select throws_ok(
  $$insert into drift_state (tenant_id) values ('00000000-0000-0000-0000-00000000000b')$$,
  '42501',
  null,
  'drift state is tenant isolated'
);

select lives_ok(
  $$insert into drift_tuning (tenant_id, params) values ('00000000-0000-0000-0000-00000000000a', '{"tier2_gate": 0.8}')$$,
  'app role manages its tuning parameters'
);

select set_config('app.tenant_id', '00000000-0000-0000-0000-00000000000b', true);
select is(
  (select count(*) from canon_statement_embeddings),
  0::bigint,
  'statement embeddings are tenant isolated for the app role'
);

select * from finish();
rollback;
