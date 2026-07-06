begin;
create extension if not exists pgtap with schema extensions;
set local search_path = public, extensions;

select plan(9);

insert into tenants (id, name, tier) values
  ('00000000-0000-0000-0000-00000000000a', 'Tenant A', 'growth'),
  ('00000000-0000-0000-0000-00000000000b', 'Tenant B', 'growth');

insert into connectors (id, tenant_id, source_type, status, config) values
  ('00000000-0000-0001-0000-00000000000a', '00000000-0000-0000-0000-00000000000a', 'slack', 'live', '{}');

insert into ingestion_queue (tenant_id, event) values
  ('00000000-0000-0000-0000-00000000000a', '{"external_id": "a-1"}'),
  ('00000000-0000-0000-0000-00000000000b', '{"external_id": "b-1"}');

set local role brain_worker;

select is(
  (select count(*) from ingestion_queue),
  2::bigint,
  'worker sees queue items across all tenants'
);

select lives_ok(
  $$insert into ingestion_queue (tenant_id, event) values ('00000000-0000-0000-0000-00000000000b', '{"external_id": "b-2"}')$$,
  'worker enqueues for any tenant'
);

select lives_ok(
  $$insert into ingestion_dlq (tenant_id, event, reason, attempts, enqueued_at) values ('00000000-0000-0000-0000-00000000000a', '{}', 'acl_invalid', 1, now())$$,
  'worker writes dead letters'
);

select set_config('app.tenant_id', '00000000-0000-0000-0000-00000000000a', true);

select lives_ok(
  $$insert into events (tenant_id, connector_id, source_type, external_id, occurred_at, acl, payload_ref) values ('00000000-0000-0000-0000-00000000000a', '00000000-0000-0001-0000-00000000000a', 'slack', 'msg-a-1', now(), '{"scope": "tenant"}', 'payloads/a/1')$$,
  'worker writes an event for the session tenant'
);

select throws_ok(
  $$insert into events (tenant_id, connector_id, source_type, external_id, occurred_at, acl, payload_ref) values ('00000000-0000-0000-0000-00000000000b', '00000000-0000-0001-0000-00000000000a', 'slack', 'msg-b-1', now(), '{"scope": "tenant"}', 'payloads/b/1')$$,
  '42501',
  null,
  'worker cannot write an event for another tenant'
);

select throws_ok(
  $$select count(*) from events$$,
  '42501',
  null,
  'worker cannot read events'
);

select throws_ok(
  $$update events set payload_ref = 'tampered'$$,
  '42501',
  null,
  'worker cannot update events'
);

reset role;
set local role brain_app;

select throws_ok(
  $$select count(*) from ingestion_queue$$,
  '42501',
  null,
  'app role cannot read the queue'
);

select throws_ok(
  $$select count(*) from ingestion_dlq$$,
  '42501',
  null,
  'app role cannot read the dead letter queue'
);

select * from finish();
rollback;
