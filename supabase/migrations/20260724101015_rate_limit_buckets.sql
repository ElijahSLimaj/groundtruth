create table rate_tiers (
  tier text primary key,
  capacity int not null check (capacity > 0),
  refill_per_second numeric not null check (refill_per_second > 0)
);

insert into rate_tiers (tier, capacity, refill_per_second) values
  ('standard', 30, 1),
  ('high', 100, 10),
  ('minimal', 2, 1.0 / 60);

grant select on rate_tiers to brain_app;

alter table rate_tiers enable row level security;

create policy read_all on rate_tiers
  for select to brain_app
  using (true);

create table api_key_buckets (
  key_id uuid primary key references api_keys(id) on delete cascade,
  tenant_id uuid not null references tenants(id),
  tokens numeric not null,
  updated_at timestamptz not null default now()
);

grant select, insert, update on api_key_buckets to brain_app;

alter table api_key_buckets enable row level security;

create policy tenant_isolation on api_key_buckets
  for all to brain_app
  using (tenant_id = public.app_tenant_id())
  with check (tenant_id = public.app_tenant_id());

create function public.meter_and_take(
  p_key_id uuid,
  p_tier text,
  p_tool text,
  p_person_id uuid
) returns table (allowed boolean, retry_after int)
language plpgsql
set search_path = public, extensions
as $$
declare
  v_tenant uuid := public.app_tenant_id();
  v_capacity int;
  v_refill numeric;
  v_tokens numeric;
  v_updated timestamptz;
begin
  if v_tenant is null then
    raise exception 'no tenant context';
  end if;

  select rt.capacity, rt.refill_per_second into v_capacity, v_refill
  from rate_tiers rt where rt.tier = p_tier;
  if v_capacity is null then
    select rt.capacity, rt.refill_per_second into v_capacity, v_refill
    from rate_tiers rt where rt.tier = 'standard';
  end if;

  insert into api_key_buckets (key_id, tenant_id, tokens, updated_at)
  values (p_key_id, v_tenant, v_capacity, now())
  on conflict (key_id) do nothing;

  select b.tokens, b.updated_at into v_tokens, v_updated
  from api_key_buckets b where b.key_id = p_key_id
  for update;

  v_tokens := least(
    v_capacity,
    v_tokens + extract(epoch from (now() - v_updated)) * v_refill
  );

  if v_tokens >= 1 then
    update api_key_buckets b
    set tokens = v_tokens - 1, updated_at = now()
    where b.key_id = p_key_id;

    insert into metering_events (tenant_id, api_key_id, person_id, tool)
    values (v_tenant, p_key_id, p_person_id, p_tool);

    allowed := true;
    retry_after := 0;
  else
    update api_key_buckets b
    set tokens = v_tokens, updated_at = now()
    where b.key_id = p_key_id;

    allowed := false;
    retry_after := ceil((1 - v_tokens) / v_refill);
  end if;

  return next;
end;
$$;

grant execute on function public.meter_and_take to brain_app;
