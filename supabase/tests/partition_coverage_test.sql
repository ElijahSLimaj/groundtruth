begin;
create extension if not exists pgtap with schema extensions;
set local search_path = public, extensions;

select plan(4);

create function pg_temp.covers_month(parent text, month_offset int) returns boolean as $fn$
  select exists (
    select 1
    from pg_class c
    join pg_namespace n on n.oid = c.relnamespace
    where n.nspname = 'public'
      and c.relispartition
      and c.relname = parent || '_' || to_char(
        date_trunc('month', current_date) + make_interval(months => month_offset),
        'YYYY_MM'
      )
  )
$fn$ language sql;

select ok(
  pg_temp.covers_month('events', 3)
  and pg_temp.covers_month('event_chunks', 3)
  and pg_temp.covers_month('audit_log', 3),
  'all three partitioned parents cover at least three months forward'
);

select ok(
  pg_temp.covers_month('events', 0)
  and pg_temp.covers_month('event_chunks', 0)
  and pg_temp.covers_month('audit_log', 0),
  'all three partitioned parents cover the current month'
);

select public.extend_partitions(3);

select ok(
  pg_temp.covers_month('events', 3),
  'extend_partitions is idempotent, a second run over existing months succeeds'
);

select throws_ok(
  $$select public.create_monthly_partitions('tenants', current_date, 1)$$,
  'create_monthly_partitions refuses parent tenants',
  'the security definer partition helper refuses parents outside the partitioned set'
);

select * from finish();
rollback;
