begin;
create extension if not exists pgtap with schema extensions;
set local search_path = public, extensions;

select plan(8);

create function pg_temp.tvec(variadic positions int[]) returns extensions.vector as $fn$
  select ('[' || string_agg(case when i = any(positions) then '1' else '0' end, ',') || ']')::extensions.vector
  from generate_series(1, 1536) i
$fn$ language sql;

select public.create_monthly_partitions('events', (date_trunc('month', now()) - interval '1 month')::date, 3);

insert into tenants (id, name, tier) values
  ('00000000-0000-0000-0000-0000000000cc', 'Gap Tenant', 'growth'),
  ('00000000-0000-0000-0000-0000000000cd', 'Other Tenant', 'growth');

insert into connectors (id, tenant_id, source_type, status, config) values
  ('00000000-0000-0001-0000-0000000000cc', '00000000-0000-0000-0000-0000000000cc', 'slack', 'live', '{}');

insert into events (id, tenant_id, connector_id, source_type, external_id, occurred_at, acl, payload_ref)
select
  ('00000000-0000-0002-00' || lpad(n::text, 2, '0') || '-0000000000cc')::uuid,
  '00000000-0000-0000-0000-0000000000cc',
  '00000000-0000-0001-0000-0000000000cc',
  'slack', 'gap-' || n, now() - interval '1 day', '{"scope": "tenant"}', 'payloads/gap/' || n
from generate_series(1, 12) n;

insert into event_chunks (id, tenant_id, event_id, event_occurred_at, chunk_index, content, embedding, embedding_model, acl, token_count, window_key, member_event_ids, source_type, tombstoned)
select
  fixture.chunk_id::uuid, '00000000-0000-0000-0000-0000000000cc', fixture.event_id::uuid, ev.occurred_at, 0,
  fixture.content, fixture.embedding, 'test-model', '{"scope": "tenant"}', 4, fixture.chunk_id,
  array[fixture.event_id::uuid], 'slack', fixture.tombstoned
from (values
  ('00000000-0000-0003-0001-0000000000cc', '00000000-0000-0002-0001-0000000000cc', 'refund topic one', pg_temp.tvec(1), false),
  ('00000000-0000-0003-0002-0000000000cc', '00000000-0000-0002-0002-0000000000cc', 'refund topic two', pg_temp.tvec(1), false),
  ('00000000-0000-0003-0003-0000000000cc', '00000000-0000-0002-0003-0000000000cc', 'refund topic three', pg_temp.tvec(1), false),
  ('00000000-0000-0003-0004-0000000000cc', '00000000-0000-0002-0004-0000000000cc', 'refund topic tombstoned', pg_temp.tvec(1), true),
  ('00000000-0000-0003-0005-0000000000cc', '00000000-0000-0002-0005-0000000000cc', 'solo author topic one', pg_temp.tvec(2), false),
  ('00000000-0000-0003-0006-0000000000cc', '00000000-0000-0002-0006-0000000000cc', 'solo author topic two', pg_temp.tvec(2), false),
  ('00000000-0000-0003-0007-0000000000cc', '00000000-0000-0002-0007-0000000000cc', 'solo author topic three', pg_temp.tvec(2), false),
  ('00000000-0000-0003-0008-0000000000cc', '00000000-0000-0002-0008-0000000000cc', 'stale topic one', pg_temp.tvec(3), false),
  ('00000000-0000-0003-0009-0000000000cc', '00000000-0000-0002-0009-0000000000cc', 'stale topic two', pg_temp.tvec(3), false),
  ('00000000-0000-0003-0010-0000000000cc', '00000000-0000-0002-0010-0000000000cc', 'stale topic three', pg_temp.tvec(3), false),
  ('00000000-0000-0003-0011-0000000000cc', '00000000-0000-0002-0011-0000000000cc', 'chain start', pg_temp.tvec(10), false),
  ('00000000-0000-0003-0012-0000000000cc', '00000000-0000-0002-0012-0000000000cc', 'chain middle', pg_temp.tvec(10, 11), false)
) as fixture(chunk_id, event_id, content, embedding, tombstoned)
join events ev on ev.id = fixture.event_id::uuid;

insert into unmatched_chunks (tenant_id, chunk_id, event_id, author_key, added_at)
select
  '00000000-0000-0000-0000-0000000000cc',
  ('00000000-0000-0003-00' || lpad(n::text, 2, '0') || '-0000000000cc')::uuid,
  ('00000000-0000-0002-00' || lpad(n::text, 2, '0') || '-0000000000cc')::uuid,
  case
    when n between 5 and 7 then 'slack:LONER'
    else 'slack:U' || n
  end,
  case
    when n between 8 and 10 then now() - interval '40 days'
    else now() - interval '1 day'
  end
from generate_series(1, 12) n;

set local role brain_app;
select set_config('app.tenant_id', '00000000-0000-0000-0000-0000000000cc', true);

select is(
  (select count(*) from public.gap_clusters(0.9, 3, 2, 30)),
  3::bigint,
  'only the diverse fresh cluster qualifies'
);

select is(
  (select count(distinct cluster_root) from public.gap_clusters(0.9, 3, 2, 30)),
  1::bigint,
  'the qualifying chunks share one cluster root'
);

select is(
  (select count(*) from public.gap_clusters(0.9, 3, 2, 30)
   where chunk_id = '00000000-0000-0003-0004-0000000000cc'),
  0::bigint,
  'tombstoned chunks never cluster'
);

select is(
  (select count(*) from public.gap_clusters(0.9, 3, 2, 30)
   where chunk_id in ('00000000-0000-0003-0005-0000000000cc', '00000000-0000-0003-0006-0000000000cc', '00000000-0000-0003-0007-0000000000cc')),
  0::bigint,
  'single author clusters are excluded'
);

select is(
  (select count(*) from public.gap_clusters(0.9, 3, 2, 30)
   where chunk_id in ('00000000-0000-0003-0008-0000000000cc', '00000000-0000-0003-0009-0000000000cc', '00000000-0000-0003-0010-0000000000cc')),
  0::bigint,
  'chunks past the buffer window are excluded'
);

select is(
  (select count(*) from public.gap_clusters(0.9, 3, 1, 30)
   where chunk_id in ('00000000-0000-0003-0005-0000000000cc', '00000000-0000-0003-0006-0000000000cc', '00000000-0000-0003-0007-0000000000cc')),
  3::bigint,
  'lowering the author threshold admits the solo cluster'
);

select is(
  (select count(*) from public.gap_clusters(0.7, 2, 2, 30)
   where chunk_id in ('00000000-0000-0003-0011-0000000000cc', '00000000-0000-0003-0012-0000000000cc')),
  2::bigint,
  'transitive similarity chains into one cluster'
);

select set_config('app.tenant_id', '00000000-0000-0000-0000-0000000000cd', true);
select is(
  (select count(*) from public.gap_clusters(0.9, 1, 1, 30)),
  0::bigint,
  'clusters are tenant isolated'
);

select * from finish();
rollback;
