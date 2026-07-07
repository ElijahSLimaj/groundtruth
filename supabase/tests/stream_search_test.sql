begin;
create extension if not exists pgtap with schema extensions;
set local search_path = public, extensions;

select plan(12);

create function pg_temp.tvec(pos int) returns extensions.vector as $fn$
  select ('[' || string_agg(case when i = pos then '1' else '0' end, ',') || ']')::extensions.vector
  from generate_series(1, 1536) i
$fn$ language sql;

select public.create_monthly_partitions('events', (date_trunc('month', now()) - interval '3 months')::date, 3);

insert into tenants (id, name, tier) values
  ('00000000-0000-0000-0000-00000000000a', 'Tenant A', 'growth'),
  ('00000000-0000-0000-0000-00000000000b', 'Tenant B', 'growth');

insert into connectors (id, tenant_id, source_type, status, config) values
  ('00000000-0000-0001-0000-00000000000a', '00000000-0000-0000-0000-00000000000a', 'slack', 'live', '{}'),
  ('00000000-0000-0001-0000-00000000000b', '00000000-0000-0000-0000-00000000000b', 'slack', 'live', '{}');

insert into events (id, tenant_id, connector_id, source_type, external_id, occurred_at, acl, payload_ref)
select
  ('00000000-0000-0002-000' || n || '-00000000000a')::uuid,
  '00000000-0000-0000-0000-00000000000a',
  '00000000-0000-0001-0000-00000000000a',
  'slack', 'e-a-' || n,
  case when n = 8 then now() - interval '60 days' else now() - interval '1 day' end,
  '{"scope": "tenant"}', 'payloads/a/' || n
from generate_series(1, 8) n;

insert into events (id, tenant_id, connector_id, source_type, external_id, occurred_at, acl, payload_ref) values
  ('00000000-0000-0002-0001-00000000000b', '00000000-0000-0000-0000-00000000000b', '00000000-0000-0001-0000-00000000000b', 'slack', 'e-b-1', now() - interval '1 day', '{"scope": "tenant"}', 'payloads/b/1');

insert into event_chunks (id, tenant_id, event_id, event_occurred_at, chunk_index, content, embedding, embedding_model, acl, token_count, window_key, member_event_ids, source_type, tombstoned)
select
  fixture.chunk_id::uuid, '00000000-0000-0000-0000-00000000000a', fixture.event_id::uuid, ev.occurred_at, 0,
  fixture.content, pg_temp.tvec(fixture.vec_pos), fixture.model, fixture.acl::jsonb, 4, fixture.window_key,
  array[fixture.event_id::uuid], 'slack', fixture.tombstoned
from (values
  ('00000000-0000-0003-0001-00000000000a', '00000000-0000-0002-0001-00000000000a', 'the growth plan pricing is 1499 per month', 1, 'test-model', '{"scope": "tenant"}', 'w-1', false),
  ('00000000-0000-0003-0002-00000000000a', '00000000-0000-0002-0002-00000000000a', 'founders only compensation discussion', 1, 'test-model', '{"scope": "principals", "principals": ["person:00000000-0000-0000-0009-00000000000x"]}', 'w-2', false),
  ('00000000-0000-0003-0004-00000000000a', '00000000-0000-0002-0004-00000000000a', 'tombstoned content that must never surface', 1, 'test-model', '{"scope": "tenant"}', 'w-4', true),
  ('00000000-0000-0003-0005-00000000000a', '00000000-0000-0002-0005-00000000000a', 'chunk from a retired embedding model', 1, 'other-model', '{"scope": "tenant"}', 'w-5', false),
  ('00000000-0000-0003-0006-00000000000a', '00000000-0000-0002-0006-00000000000a', 'the zanzibar rollout ships next week', 2, 'test-model', '{"scope": "tenant"}', 'w-6', false),
  ('00000000-0000-0003-0007-00000000000a', '00000000-0000-0002-0007-00000000000a', 'fresh note about the incident retro', 3, 'test-model', '{"scope": "tenant"}', 'w-7', false),
  ('00000000-0000-0003-0008-00000000000a', '00000000-0000-0002-0008-00000000000a', 'stale note about the incident retro', 3, 'test-model', '{"scope": "tenant"}', 'w-8', false)
) as fixture(chunk_id, event_id, content, vec_pos, model, acl, window_key, tombstoned)
join events ev on ev.id = fixture.event_id::uuid;

insert into event_chunks (id, tenant_id, event_id, event_occurred_at, chunk_index, content, embedding, embedding_model, acl, token_count, window_key, member_event_ids, source_type)
select
  '00000000-0000-0003-0001-00000000000b', '00000000-0000-0000-0000-00000000000b', id, occurred_at, 0,
  'tenant b confidential pricing', pg_temp.tvec(1), 'test-model', '{"scope": "tenant"}', 4, 'w-b1', array[id], 'slack'
from events where id = '00000000-0000-0002-0001-00000000000b';

set local role brain_app;
select set_config('app.tenant_id', '00000000-0000-0000-0000-00000000000a', true);

select ok(
  exists(select 1 from public.stream_search(pg_temp.tvec(1), 'pricing', 'test-model')
         where chunk_id = '00000000-0000-0003-0001-00000000000a'),
  'tenant scoped chunk is retrievable by any tenant member'
);

select ok(
  not exists(select 1 from public.stream_search(pg_temp.tvec(1), 'compensation', 'test-model')
             where chunk_id = '00000000-0000-0003-0002-00000000000a'),
  'principals chunk hidden from a caller without the principal'
);

select ok(
  exists(select 1 from public.stream_search(pg_temp.tvec(1), 'compensation', 'test-model',
                                            array['person:00000000-0000-0000-0009-00000000000x'])
         where chunk_id = '00000000-0000-0003-0002-00000000000a'),
  'principals chunk visible to a matching principal'
);

select ok(
  not exists(select 1 from public.stream_search(pg_temp.tvec(1), 'pricing', 'test-model')
             where chunk_id = '00000000-0000-0003-0001-00000000000b'),
  'other tenant chunks never surface even with identical embeddings'
);

select ok(
  not exists(select 1 from public.stream_search(pg_temp.tvec(1), 'tombstoned', 'test-model')
             where chunk_id = '00000000-0000-0003-0004-00000000000a'),
  'tombstoned chunks are excluded'
);

select ok(
  not exists(select 1 from public.stream_search(pg_temp.tvec(1), 'retired', 'test-model')
             where chunk_id = '00000000-0000-0003-0005-00000000000a'),
  'chunks from other embedding models are excluded'
);

select is(
  (select chunk_id from public.stream_search(pg_temp.tvec(9), 'zanzibar', 'test-model') limit 1),
  '00000000-0000-0003-0006-00000000000a'::uuid,
  'exact term match wins through the text arm despite a distant embedding'
);

select ok(
  (select array_position(array_agg(chunk_id), '00000000-0000-0003-0007-00000000000a'::uuid)
        < array_position(array_agg(chunk_id), '00000000-0000-0003-0008-00000000000a'::uuid)
   from public.stream_search(pg_temp.tvec(3), '', 'test-model')),
  'freshness decay ranks the recent chunk above the stale one at equal similarity'
);

select is(
  (select count(*) from public.stream_search(pg_temp.tvec(1), 'pricing', 'test-model', '{}', 1)),
  1::bigint,
  'k limits the result count'
);

select ok(
  (select similarity > 0.99 from public.stream_search(pg_temp.tvec(1), 'pricing', 'test-model', '{}', 1)),
  'similarity reflects cosine closeness'
);

select ok(
  not exists(select 1 from public.stream_search(pg_temp.tvec(1), 'pricing', 'test-model', '{}', 12, array['gmail'])),
  'source type filter excludes other sources'
);

select set_config('app.tenant_id', '', true);
select is(
  (select count(*) from public.stream_search(pg_temp.tvec(1), 'pricing', 'test-model')),
  0::bigint,
  'no tenant context retrieves nothing'
);

select * from finish();
rollback;
