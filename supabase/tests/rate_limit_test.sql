begin;
create extension if not exists pgtap with schema extensions;
set local search_path = public, extensions;

select plan(9);

insert into tenants (id, name, tier) values
  ('00000000-0000-0000-0000-0000000000a1', 'Tenant A', 'growth'),
  ('00000000-0000-0000-0000-0000000000b1', 'Tenant B', 'growth');

insert into people (id, tenant_id, email, display_name, role) values
  ('00000000-0000-0000-0001-0000000000a1', '00000000-0000-0000-0000-0000000000a1', 'agent@a.test', 'Agent A', 'agent'),
  ('00000000-0000-0000-0001-0000000000b1', '00000000-0000-0000-0000-0000000000b1', 'agent@b.test', 'Agent B', 'agent');

insert into api_keys (id, tenant_id, person_id, key_hash, name, rate_tier) values
  ('00000000-0000-0000-0002-0000000000a1', '00000000-0000-0000-0000-0000000000a1', '00000000-0000-0000-0001-0000000000a1', 'hash-a', 'key a', 'minimal'),
  ('00000000-0000-0000-0002-0000000000a2', '00000000-0000-0000-0000-0000000000a1', '00000000-0000-0000-0001-0000000000a1', 'hash-a2', 'key a2', 'nonsense'),
  ('00000000-0000-0000-0002-0000000000b1', '00000000-0000-0000-0000-0000000000b1', '00000000-0000-0000-0001-0000000000b1', 'hash-b', 'key b', 'standard');

set local role brain_app;
select set_config('app.tenant_id', '00000000-0000-0000-0000-0000000000a1', true);

select ok(
  (select allowed from public.meter_and_take(
    '00000000-0000-0000-0002-0000000000a1', 'minimal', '/tools/query',
    '00000000-0000-0000-0001-0000000000a1')),
  'the first call against a fresh bucket is allowed'
);

select ok(
  (select allowed from public.meter_and_take(
    '00000000-0000-0000-0002-0000000000a1', 'minimal', '/tools/query',
    '00000000-0000-0000-0001-0000000000a1')),
  'the minimal tier allows a second call at capacity two'
);

select ok(
  not (select allowed from public.meter_and_take(
    '00000000-0000-0000-0002-0000000000a1', 'minimal', '/tools/query',
    '00000000-0000-0000-0001-0000000000a1')),
  'the third call drains the bucket and is refused'
);

select ok(
  (select retry_after > 0 from public.meter_and_take(
    '00000000-0000-0000-0002-0000000000a1', 'minimal', '/tools/query',
    '00000000-0000-0000-0001-0000000000a1')),
  'a refused call reports a positive retry_after'
);

select is(
  (select count(*) from metering_events where api_key_id = '00000000-0000-0000-0002-0000000000a1'),
  2::bigint,
  'only allowed calls meter, refused calls never bill'
);

select ok(
  (select bool_and(category = 'tool_call' and billable)
   from metering_events where api_key_id = '00000000-0000-0000-0002-0000000000a1'),
  'the agent key path defaults to a billable tool_call category under the enriched schema'
);

update api_key_buckets
set updated_at = now() - interval '10 minutes'
where key_id = '00000000-0000-0000-0002-0000000000a1';

select ok(
  (select allowed from public.meter_and_take(
    '00000000-0000-0000-0002-0000000000a1', 'minimal', '/tools/query',
    '00000000-0000-0000-0001-0000000000a1')),
  'the bucket refills over elapsed time at the tier rate'
);

select ok(
  (select allowed from public.meter_and_take(
    '00000000-0000-0000-0002-0000000000a2', 'nonsense', '/tools/query',
    '00000000-0000-0000-0001-0000000000a1')),
  'an unknown rate tier falls back to standard rather than failing open or closed'
);

select set_config('app.tenant_id', '00000000-0000-0000-0000-0000000000b1', true);

select is(
  (select count(*) from api_key_buckets),
  0::bigint,
  'buckets are tenant isolated, tenant b cannot see tenant a usage'
);

select * from finish();
rollback;
