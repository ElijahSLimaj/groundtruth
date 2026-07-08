begin;
create extension if not exists pgtap with schema extensions;
set local search_path = public, extensions;

select plan(17);

select public.create_monthly_partitions('events', (date_trunc('month', now()) - interval '1 month')::date, 3);

insert into tenants (id, name, tier) values
  ('00000000-0000-0000-0000-0000000000ee', 'Erasure Tenant', 'growth');

insert into people (id, tenant_id, email, display_name, role) values
  ('00000000-0000-0000-0001-0000000000ee', '00000000-0000-0000-0000-0000000000ee', 'admin@e.test', 'Admin', 'admin'),
  ('00000000-0000-0000-0002-0000000000ee', '00000000-0000-0000-0000-0000000000ee', 'victim@e.test', 'Victim', 'member'),
  ('00000000-0000-0000-0003-0000000000ee', '00000000-0000-0000-0000-0000000000ee', 'bystander@e.test', 'Bystander', 'member');

insert into connectors (id, tenant_id, source_type, status, config) values
  ('00000000-0000-0001-0000-0000000000ee', '00000000-0000-0000-0000-0000000000ee', 'slack', 'live', '{}');

insert into events (id, tenant_id, connector_id, source_type, external_id, author_id, occurred_at, acl, payload_ref) values
  ('00000000-0000-0002-0001-0000000000ee', '00000000-0000-0000-0000-0000000000ee', '00000000-0000-0001-0000-0000000000ee', 'slack', 'ev-1', '00000000-0000-0000-0002-0000000000ee', now() - interval '2 days', '{"scope": "tenant"}', 'payloads/00000000-0000-0000-0000-0000000000ee/aaaa'),
  ('00000000-0000-0002-0002-0000000000ee', '00000000-0000-0000-0000-0000000000ee', '00000000-0000-0001-0000-0000000000ee', 'slack', 'ev-2', '00000000-0000-0000-0002-0000000000ee', now() - interval '1 day', '{"scope": "tenant"}', 'payloads/00000000-0000-0000-0000-0000000000ee/shared'),
  ('00000000-0000-0002-0003-0000000000ee', '00000000-0000-0000-0000-0000000000ee', '00000000-0000-0001-0000-0000000000ee', 'slack', 'ev-3', '00000000-0000-0000-0003-0000000000ee', now() - interval '1 day', '{"scope": "tenant"}', 'payloads/00000000-0000-0000-0000-0000000000ee/shared'),
  ('00000000-0000-0002-0004-0000000000ee', '00000000-0000-0000-0000-0000000000ee', '00000000-0000-0001-0000-0000000000ee', 'slack', 'ev-4', '00000000-0000-0000-0003-0000000000ee', now() - interval '1 day', '{"scope": "tenant"}', 'payloads/00000000-0000-0000-0000-0000000000ee/bbbb');

insert into event_chunks (id, tenant_id, event_id, event_occurred_at, chunk_index, content, embedding, embedding_model, acl, token_count, window_key, member_event_ids, source_type)
select
  fixture.chunk_id::uuid, '00000000-0000-0000-0000-0000000000ee', fixture.event_id::uuid, ev.occurred_at, 0,
  fixture.content, array_fill(0.5, array[1536])::vector, 'test-model', '{"scope": "tenant"}', 4, fixture.window_key,
  array[fixture.event_id::uuid], 'slack'
from (values
  ('00000000-0000-0003-0001-0000000000ee', '00000000-0000-0002-0001-0000000000ee', 'victim message one', 'w-1'),
  ('00000000-0000-0003-0002-0000000000ee', '00000000-0000-0002-0002-0000000000ee', 'victim message two', 'w-2'),
  ('00000000-0000-0003-0003-0000000000ee', '00000000-0000-0002-0003-0000000000ee', 'bystander message', 'w-3'),
  ('00000000-0000-0003-0004-0000000000ee', '00000000-0000-0002-0004-0000000000ee', 'bystander deleted message', 'w-4')
) as fixture(chunk_id, event_id, content, window_key)
join events ev on ev.id = fixture.event_id::uuid;

insert into canon_entries (id, tenant_id, domain, tier, owner_id, status, visibility, verify_interval) values
  ('00000000-0000-0004-0001-0000000000ee', '00000000-0000-0000-0000-0000000000ee', 'pricing', 'operational', '00000000-0000-0000-0001-0000000000ee', 'active', '{"scope": "tenant"}', interval '60 days');

insert into canon_versions (id, tenant_id, entry_id, version_number, statement, created_by, status) values
  ('00000000-0000-0005-0001-0000000000ee', '00000000-0000-0000-0000-0000000000ee', '00000000-0000-0004-0001-0000000000ee', 1, 'Fact sourced from the victim', '00000000-0000-0000-0001-0000000000ee', 'approved');

insert into canon_provenance (tenant_id, version_id, event_id, event_occurred_at)
select '00000000-0000-0000-0000-0000000000ee', '00000000-0000-0005-0001-0000000000ee', id, occurred_at
from events where id = '00000000-0000-0002-0001-0000000000ee';

insert into api_keys (tenant_id, person_id, key_hash, name) values
  ('00000000-0000-0000-0000-0000000000ee', '00000000-0000-0000-0002-0000000000ee', 'victimhash', 'victim key');

set local role brain_app;
select set_config('app.tenant_id', '00000000-0000-0000-0000-0000000000ee', true);

select is(
  public.event_tombstone('00000000-0000-0002-0004-0000000000ee', '00000000-0000-0000-0001-0000000000ee'),
  1,
  'tombstoning an event flags its chunks'
);

select is(
  (select tombstoned from events where id = '00000000-0000-0002-0004-0000000000ee'),
  true,
  'the event carries the tombstone flag'
);

select throws_ok(
  $$select public.event_tombstone('00000000-0000-0002-0004-0000000000ee', '00000000-0000-0000-0001-0000000000ee')$$,
  'event 00000000-0000-0002-0004-0000000000ee not found or already tombstoned',
  'tombstoning twice fails loudly'
);

insert into erasure_requests (id, tenant_id, person_id, requested_by, reason) values
  ('00000000-0000-0006-0001-0000000000ee', '00000000-0000-0000-0000-0000000000ee', '00000000-0000-0000-0002-0000000000ee', '00000000-0000-0000-0001-0000000000ee', 'gdpr article 17');

select throws_ok(
  $$select public.erasure_execute('00000000-0000-0006-0001-0000000000ee', '00000000-0000-0000-0001-0000000000ee')$$,
  'erasure request 00000000-0000-0006-0001-0000000000ee is pending, only verified requests can be executed',
  'unverified requests cannot execute'
);

update erasure_requests set status = 'verified', verified_by = '00000000-0000-0000-0001-0000000000ee', verified_at = now()
where id = '00000000-0000-0006-0001-0000000000ee';

select throws_ok(
  $$select public.erasure_execute('00000000-0000-0006-0001-0000000000ee', '00000000-0000-0000-0003-0000000000ee')$$,
  'executing an erasure requires the admin role',
  'members cannot execute erasures'
);

select results_eq(
  $$select payload_ref from public.erasure_execute('00000000-0000-0006-0001-0000000000ee', '00000000-0000-0000-0001-0000000000ee')$$,
  $$values ('payloads/00000000-0000-0000-0000-0000000000ee/aaaa')$$,
  'execution returns only payload refs with no live references'
);

select is(
  (select count(*) from events where author_id = '00000000-0000-0000-0002-0000000000ee' and not tombstoned),
  0::bigint,
  'all authored events are tombstoned'
);

select is(
  (select count(*) from event_chunks where event_id in ('00000000-0000-0002-0001-0000000000ee', '00000000-0000-0002-0002-0000000000ee')),
  0::bigint,
  'authored chunks are physically removed'
);

select is(
  (select count(*) from event_chunks where event_id = '00000000-0000-0002-0003-0000000000ee'),
  1::bigint,
  'other authors keep their chunks'
);

select is(
  (select erased from canon_provenance where version_id = '00000000-0000-0005-0001-0000000000ee'),
  true,
  'canon provenance citing erased events is marked erased'
);

select is(
  (select status from canon_entries where id = '00000000-0000-0004-0001-0000000000ee'),
  'active',
  'canon entries citing erased events survive'
);

select is(
  (select email from people where id = '00000000-0000-0000-0002-0000000000ee'),
  'erased+00000000-0000-0000-0002-0000000000ee@erased.invalid',
  'the person identity is anonymized'
);

select is(
  (select display_name from people where id = '00000000-0000-0000-0002-0000000000ee'),
  'Erased person',
  'the display name is scrubbed'
);

select is(
  (select count(*) from api_keys where person_id = '00000000-0000-0000-0002-0000000000ee' and revoked_at is null),
  0::bigint,
  'the erased person keys are revoked'
);

select is(
  (select status from erasure_requests where id = '00000000-0000-0006-0001-0000000000ee'),
  'completed',
  'the request completes'
);

select is(
  (select count(*) from audit_log where action = 'person.erased'),
  1::bigint,
  'erasure is audited'
);

select throws_ok(
  $$select public.erasure_execute('00000000-0000-0006-0001-0000000000ee', '00000000-0000-0000-0001-0000000000ee')$$,
  'erasure request 00000000-0000-0006-0001-0000000000ee is completed, only verified requests can be executed',
  'completed requests cannot re-execute'
);

select * from finish();
rollback;
