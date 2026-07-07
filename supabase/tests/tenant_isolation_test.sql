begin;
create extension if not exists pgtap with schema extensions;
set local search_path = public, extensions;

select plan(22);

insert into tenants (id, name, tier) values
  ('00000000-0000-0000-0000-00000000000a', 'Tenant A', 'growth'),
  ('00000000-0000-0000-0000-00000000000b', 'Tenant B', 'growth');

insert into people (id, tenant_id, email, display_name, role) values
  ('00000000-0000-0000-0001-00000000000a', '00000000-0000-0000-0000-00000000000a', 'owner@a.test', 'Owner A', 'owner'),
  ('00000000-0000-0000-0002-00000000000a', '00000000-0000-0000-0000-00000000000a', 'member@a.test', 'Member A', 'member'),
  ('00000000-0000-0000-0001-00000000000b', '00000000-0000-0000-0000-00000000000b', 'owner@b.test', 'Owner B', 'owner');

insert into connectors (id, tenant_id, source_type, status, config) values
  ('00000000-0000-0001-0000-00000000000a', '00000000-0000-0000-0000-00000000000a', 'slack', 'live', '{}'),
  ('00000000-0000-0001-0000-00000000000b', '00000000-0000-0000-0000-00000000000b', 'slack', 'live', '{}');

insert into events (id, tenant_id, connector_id, source_type, external_id, occurred_at, acl, payload_ref) values
  ('00000000-0000-0002-0001-00000000000a', '00000000-0000-0000-0000-00000000000a', '00000000-0000-0001-0000-00000000000a', 'slack', 'msg-a-1', now(), '{"scope": "tenant"}', 'payloads/a/1'),
  ('00000000-0000-0002-0002-00000000000a', '00000000-0000-0000-0000-00000000000a', '00000000-0000-0001-0000-00000000000a', 'slack', 'msg-a-2', now(), '{"scope": "tenant"}', 'payloads/a/2'),
  ('00000000-0000-0002-0001-00000000000b', '00000000-0000-0000-0000-00000000000b', '00000000-0000-0001-0000-00000000000b', 'slack', 'msg-b-1', now(), '{"scope": "tenant"}', 'payloads/b/1');

insert into event_chunks (tenant_id, event_id, event_occurred_at, chunk_index, content, embedding, embedding_model, acl, token_count, window_key, member_event_ids)
select
  '00000000-0000-0000-0000-00000000000a',
  '00000000-0000-0002-0001-00000000000a',
  occurred_at,
  0,
  'chunk content a',
  array_fill(0.1, array[1536])::vector,
  'test-model',
  '{"scope": "tenant"}',
  4,
  'thread-a',
  array['00000000-0000-0002-0001-00000000000a']::uuid[]
from events where id = '00000000-0000-0002-0001-00000000000a';

insert into canon_entries (id, tenant_id, domain, tier, owner_id, status, visibility, verify_interval) values
  ('00000000-0000-0003-0000-00000000000a', '00000000-0000-0000-0000-00000000000a', 'pricing', 'bedrock', '00000000-0000-0000-0001-00000000000a', 'active', '{"scope": "tenant"}', interval '90 days'),
  ('00000000-0000-0003-0000-00000000000b', '00000000-0000-0000-0000-00000000000b', 'pricing', 'bedrock', '00000000-0000-0000-0001-00000000000b', 'active', '{"scope": "tenant"}', interval '90 days');

insert into canon_versions (id, tenant_id, entry_id, version_number, statement, created_by) values
  ('00000000-0000-0004-0000-00000000000a', '00000000-0000-0000-0000-00000000000a', '00000000-0000-0003-0000-00000000000a', 1, 'Statement A', '00000000-0000-0000-0001-00000000000a'),
  ('00000000-0000-0004-0000-00000000000b', '00000000-0000-0000-0000-00000000000b', '00000000-0000-0003-0000-00000000000b', 1, 'Statement B', '00000000-0000-0000-0001-00000000000b');

insert into audit_log (tenant_id, actor_id, action, subject_type, subject_id) values
  ('00000000-0000-0000-0000-00000000000a', '00000000-0000-0000-0001-00000000000a', 'canon.approve', 'canon_version', '00000000-0000-0004-0000-00000000000a'),
  ('00000000-0000-0000-0000-00000000000b', '00000000-0000-0000-0001-00000000000b', 'canon.approve', 'canon_version', '00000000-0000-0004-0000-00000000000b');

set local role brain_app;
select set_config('app.tenant_id', '00000000-0000-0000-0000-00000000000a', true);

select is((select count(*) from tenants), 1::bigint, 'tenant A sees exactly one tenants row');
select is((select id from tenants), '00000000-0000-0000-0000-00000000000a'::uuid, 'tenant A sees only its own tenant');
select is((select count(*) from people), 2::bigint, 'tenant A sees only its own people');
select is((select count(*) from connectors), 1::bigint, 'tenant A sees only its own connectors');
select is((select count(*) from events), 2::bigint, 'tenant A sees only its own events');
select is((select count(*) from event_chunks), 1::bigint, 'tenant A sees only its own event chunks');
select is((select count(*) from canon_entries), 1::bigint, 'tenant A sees only its own canon entries');
select is((select count(*) from canon_versions), 1::bigint, 'tenant A sees only its own canon versions');
select is((select count(*) from audit_log), 1::bigint, 'tenant A sees only its own audit rows');
select is((select count(*) from events where tenant_id = '00000000-0000-0000-0000-00000000000b'), 0::bigint, 'explicit filter for tenant B returns nothing');

select throws_ok(
  $$insert into people (tenant_id, email, display_name, role) values ('00000000-0000-0000-0000-00000000000b', 'intruder@b.test', 'Intruder', 'member')$$,
  '42501',
  null,
  'insert into another tenant is rejected'
);

select throws_ok(
  $$insert into events (tenant_id, connector_id, source_type, external_id, occurred_at, acl, payload_ref) values ('00000000-0000-0000-0000-00000000000b', '00000000-0000-0001-0000-00000000000b', 'slack', 'msg-b-2', now(), '{"scope": "tenant"}', 'payloads/b/2')$$,
  '42501',
  null,
  'insert event into another tenant is rejected'
);

select throws_ok(
  $$update events set payload_ref = 'tampered'$$,
  '42501',
  null,
  'events are append only, update denied'
);

select throws_ok(
  $$delete from events$$,
  '42501',
  null,
  'events are append only, delete denied'
);

select throws_ok(
  $$update audit_log set action = 'tampered'$$,
  '42501',
  null,
  'audit log is append only, update denied'
);

select throws_ok(
  $$delete from audit_log$$,
  '42501',
  null,
  'audit log is append only, delete denied'
);

select lives_ok(
  $$insert into events (tenant_id, connector_id, source_type, external_id, occurred_at, acl, payload_ref) values ('00000000-0000-0000-0000-00000000000a', '00000000-0000-0001-0000-00000000000a', 'slack', 'msg-a-3', now(), '{"scope": "tenant"}', 'payloads/a/3')$$,
  'insert into own tenant succeeds'
);

select lives_ok(
  $$insert into audit_log (tenant_id, actor_id, action, subject_type) values ('00000000-0000-0000-0000-00000000000a', '00000000-0000-0000-0001-00000000000a', 'test.write', 'test')$$,
  'audit insert into own tenant succeeds'
);

select set_config('app.tenant_id', '00000000-0000-0000-0000-00000000000b', true);
select is((select count(*) from events), 1::bigint, 'tenant B sees only its own events');
select is((select count(*) from canon_versions), 1::bigint, 'tenant B sees only its own canon versions');

select set_config('app.tenant_id', '', true);
select is((select count(*) from tenants), 0::bigint, 'no tenant context sees no tenants');
select is((select count(*) from events), 0::bigint, 'no tenant context sees no events');

select * from finish();
rollback;
