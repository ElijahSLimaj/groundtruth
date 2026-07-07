begin;
create extension if not exists pgtap with schema extensions;
set local search_path = public, extensions;

select plan(9);

insert into tenants (id, name, tier) values
  ('00000000-0000-0000-0000-00000000000a', 'Tenant A', 'growth'),
  ('00000000-0000-0000-0000-00000000000b', 'Tenant B', 'growth');

insert into connectors (id, tenant_id, source_type, status, config) values
  ('00000000-0000-0001-0000-00000000000a', '00000000-0000-0000-0000-00000000000a', 'slack', 'live', '{}'),
  ('00000000-0000-0001-0000-00000000000b', '00000000-0000-0000-0000-00000000000b', 'slack', 'live', '{}');

insert into events (id, tenant_id, connector_id, source_type, external_id, occurred_at, acl, payload_ref) values
  ('00000000-0000-0002-0001-00000000000a', '00000000-0000-0000-0000-00000000000a', '00000000-0000-0001-0000-00000000000a', 'slack', 'e-a', now(), '{"scope": "tenant"}', 'payloads/a/1'),
  ('00000000-0000-0002-0001-00000000000b', '00000000-0000-0000-0000-00000000000b', '00000000-0000-0001-0000-00000000000b', 'slack', 'e-b', now(), '{"scope": "tenant"}', 'payloads/b/1');

set local role brain_embedder;

select is(
  (select count(*) from events where id in ('00000000-0000-0002-0001-00000000000a', '00000000-0000-0002-0001-00000000000b')),
  2::bigint,
  'embedder reads events across all tenants'
);

select lives_ok(
  $$insert into event_chunks (tenant_id, event_id, event_occurred_at, chunk_index, content, embedding, embedding_model, acl, token_count, window_key, member_event_ids, source_type)
    select '00000000-0000-0000-0000-00000000000a', id, occurred_at, 0, 'chunk', array_fill(0.5, array[1536])::vector, 'test-model', acl, 2, 'w-1', array[id], 'slack'
    from events where id = '00000000-0000-0002-0001-00000000000a'$$,
  'embedder writes chunks'
);

select lives_ok(
  $$delete from event_chunks where event_id = '00000000-0000-0002-0001-00000000000a'$$,
  'embedder rebuilds chunks via delete'
);

select lives_ok(
  $$insert into embedding_watermark (embedding_model, last_ingested_at) values ('test-model', now())
    on conflict (embedding_model) do update set last_ingested_at = excluded.last_ingested_at, updated_at = now()$$,
  'embedder upserts its watermark'
);

select lives_ok(
  $$insert into embedding_dlq (tenant_id, event_id, chunk_index, reason) values ('00000000-0000-0000-0000-00000000000a', '00000000-0000-0002-0001-00000000000a', 0, 'provider rejected content')$$,
  'embedder dead letters poisoned chunks'
);

select throws_ok(
  $$update events set payload_ref = 'tampered' where id = '00000000-0000-0002-0001-00000000000a'$$,
  '42501',
  null,
  'embedder cannot mutate events'
);

select throws_ok(
  $$select count(*) from ingestion_queue$$,
  '42501',
  null,
  'embedder cannot read the ingestion queue'
);

reset role;
set local role brain_app;

select throws_ok(
  $$select count(*) from embedding_watermark$$,
  '42501',
  null,
  'app role cannot read the watermark'
);

select throws_ok(
  $$select count(*) from embedding_dlq$$,
  '42501',
  null,
  'app role cannot read the embedding dlq'
);

select * from finish();
rollback;
