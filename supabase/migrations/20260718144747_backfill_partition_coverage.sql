select public.create_monthly_partitions(
  'events', (date_trunc('month', current_date) - interval '18 months')::date, 30);
select public.create_monthly_partitions(
  'event_chunks', (date_trunc('month', current_date) - interval '18 months')::date, 30);
select public.create_monthly_partitions(
  'audit_log', (date_trunc('month', current_date) - interval '18 months')::date, 30);
