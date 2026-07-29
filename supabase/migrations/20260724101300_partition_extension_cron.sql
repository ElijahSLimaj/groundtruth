create or replace function public.create_monthly_partitions(
  parent regclass,
  from_month date,
  months_ahead int
) returns void
language plpgsql
security definer
set search_path = public, pg_temp
as $$
declare
  month_start date;
  partition_name text;
begin
  if parent::text not in (
    'events', 'event_chunks', 'audit_log',
    'public.events', 'public.event_chunks', 'public.audit_log'
  ) then
    raise exception 'create_monthly_partitions refuses parent %', parent;
  end if;

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

revoke all on function public.create_monthly_partitions(regclass, date, int) from public;

create function public.extend_partitions(p_months_ahead int default 3) returns void
language plpgsql
security definer
set search_path = public, pg_temp
as $$
begin
  perform public.create_monthly_partitions('events', current_date, p_months_ahead);
  perform public.create_monthly_partitions('event_chunks', current_date, p_months_ahead);
  perform public.create_monthly_partitions('audit_log', current_date, p_months_ahead);
end;
$$;

revoke all on function public.extend_partitions(int) from public;
grant execute on function public.extend_partitions(int) to brain_app;

select public.extend_partitions(24);

do $$
begin
  create extension if not exists pg_cron;
  perform cron.schedule(
    'extend-partitions',
    '0 3 1 * *',
    'select public.extend_partitions(3)'
  );
exception when others then
  raise notice 'pg_cron unavailable (%), partition extension falls back to the serving scheduler', sqlerrm;
end;
$$;
