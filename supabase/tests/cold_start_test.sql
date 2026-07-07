begin;
create extension if not exists pgtap with schema extensions;
set local search_path = public, extensions;

select plan(3);

insert into tenants (id, name, tier) values
  ('00000000-0000-0000-0000-00000000000a', 'Tenant A', 'growth'),
  ('00000000-0000-0000-0000-00000000000b', 'Tenant B', 'growth');

set local role brain_app;
select set_config('app.tenant_id', '00000000-0000-0000-0000-00000000000a', true);

select lives_ok(
  $$insert into cold_start_state (tenant_id, llm_calls) values ('00000000-0000-0000-0000-00000000000a', 3)$$,
  'app role manages its cold start watermark'
);

select throws_ok(
  $$insert into cold_start_state (tenant_id) values ('00000000-0000-0000-0000-00000000000b')$$,
  '42501',
  null,
  'cold start state is tenant isolated'
);

select set_config('app.tenant_id', '00000000-0000-0000-0000-00000000000b', true);
select is(
  (select count(*) from cold_start_state),
  0::bigint,
  'other tenants see no cold start state'
);

select * from finish();
rollback;
