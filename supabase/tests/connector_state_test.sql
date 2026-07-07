begin;
create extension if not exists pgtap with schema extensions;
set local search_path = public, extensions;

select plan(8);

insert into tenants (id, name, tier) values
  ('00000000-0000-0000-0000-00000000000a', 'Tenant A', 'growth'),
  ('00000000-0000-0000-0000-00000000000b', 'Tenant B', 'growth');

insert into connectors (id, tenant_id, source_type, status, config) values
  ('00000000-0000-0001-0000-00000000000a', '00000000-0000-0000-0000-00000000000a', 'slack', 'live', '{"token": "secret-a"}'),
  ('00000000-0000-0001-0000-00000000000b', '00000000-0000-0000-0000-00000000000b', 'gmail', 'live', '{"token": "secret-b"}');

set local role brain_worker;

select is(
  (select count(*) from connectors where id in ('00000000-0000-0001-0000-00000000000a', '00000000-0000-0001-0000-00000000000b')),
  2::bigint,
  'worker reads connectors across all tenants'
);

select lives_ok(
  $$insert into connector_state (connector_id, tenant_id, poll_cursor) values ('00000000-0000-0001-0000-00000000000a', '00000000-0000-0000-0000-00000000000a', 'c-1')$$,
  'worker persists a cursor'
);

select lives_ok(
  $$insert into connector_state (connector_id, tenant_id, poll_cursor) values ('00000000-0000-0001-0000-00000000000a', '00000000-0000-0000-0000-00000000000a', 'c-2') on conflict (connector_id) do update set poll_cursor = excluded.poll_cursor, updated_at = now()$$,
  'worker upserts a cursor'
);

select is(
  (select poll_cursor from connector_state where connector_id = '00000000-0000-0001-0000-00000000000a'),
  'c-2',
  'cursor upsert took effect'
);

select lives_ok(
  $$update connectors set status = 'degraded' where id = '00000000-0000-0001-0000-00000000000a'$$,
  'worker flips connector status'
);

select throws_ok(
  $$update connectors set config = '{}' where id = '00000000-0000-0001-0000-00000000000a'$$,
  '42501',
  null,
  'worker cannot rewrite connector config'
);

reset role;
set local role brain_app;

select is(
  (select count(*) from connector_state),
  0::bigint,
  'app role sees no cursors without tenant context'
);

select set_config('app.tenant_id', '00000000-0000-0000-0000-00000000000a', true);
select is(
  (select count(*) from connector_state),
  1::bigint,
  'app role reads only its own tenant cursors'
);

select * from finish();
rollback;
