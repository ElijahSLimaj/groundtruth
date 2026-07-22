grant select on tenant_keys to brain_embedder;

create policy embedder_read_tenant_keys on tenant_keys
  for select to brain_embedder
  using (true);

do $$
declare
  part record;
begin
  for part in
    select c.relname
    from pg_class c
    join pg_namespace n on n.oid = c.relnamespace
    where n.nspname = 'public'
      and c.relkind = 'r'
      and c.relispartition
      and not c.relrowsecurity
  loop
    execute format('alter table public.%I enable row level security', part.relname);
  end loop;
end $$;

create or replace function public.create_monthly_partitions(
  parent regclass,
  from_month date,
  months_ahead int
) returns void
language plpgsql
as $$
declare
  month_start date;
  partition_name text;
begin
  for offset_months in 0..months_ahead loop
    month_start := date_trunc('month', from_month) + make_interval(months => offset_months);
    partition_name := format(
      '%s_%s',
      replace(parent::text, 'public.', ''),
      to_char(month_start, 'YYYY_MM')
    );
    execute format(
      'create table if not exists %I partition of %s for values from (%L) to (%L)',
      partition_name,
      parent,
      month_start,
      month_start + interval '1 month'
    );
    execute format(
      'alter table %I enable row level security',
      partition_name
    );
  end loop;
end;
$$;

select public.create_monthly_partitions('events', current_date, 12);
select public.create_monthly_partitions('event_chunks', current_date, 12);
select public.create_monthly_partitions('audit_log', current_date, 12);

revoke all on all tables in schema public from anon, authenticated;
revoke all on all sequences in schema public from anon, authenticated;
