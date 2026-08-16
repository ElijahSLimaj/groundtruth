create table plan_limits (
  plan text primary key,
  connector_cap int not null,
  entry_budget int not null,
  included_query_volume int not null,
  stripe_price_monthly text,
  stripe_price_yearly text
);

insert into plan_limits (plan, connector_cap, entry_budget, included_query_volume) values
  ('core', 4, 150, 100000),
  ('growth', 100, 400, 500000),
  ('scale', 100, 1000, 2000000);

grant select on plan_limits to brain_app;

alter table plan_limits enable row level security;

create policy read_all on plan_limits
  for select to brain_app
  using (true);

alter table tenants
  add column plan text references plan_limits(plan),
  add column billing_interval text check (billing_interval in ('month', 'year')),
  add column stripe_customer_id text,
  add column stripe_subscription_id text,
  add column subscription_status text,
  add column pilot_ends_at timestamptz,
  add column included_query_volume int;

create function public.provision_tenant(
  p_email text,
  p_display_name text,
  p_company text,
  p_plan text default 'core',
  p_interval text default 'month'
) returns table (tenant_id uuid, person_id uuid, created boolean)
language plpgsql
security definer
set search_path = public
as $$
declare
  v_tenant uuid;
  v_person uuid;
  v_budget int;
  v_volume int;
begin
  select p.id, p.tenant_id into v_person, v_tenant
  from people p where lower(p.email) = lower(p_email) limit 1;
  if v_person is not null then
    return query select v_tenant, v_person, false;
    return;
  end if;

  select pl.entry_budget, pl.included_query_volume into v_budget, v_volume
  from plan_limits pl where pl.plan = p_plan;
  if v_budget is null then
    p_plan := 'core';
    select pl.entry_budget, pl.included_query_volume into v_budget, v_volume
    from plan_limits pl where pl.plan = 'core';
  end if;

  insert into tenants (name, tier, plan, billing_interval, entry_budget,
                       included_query_volume, subscription_status)
  values (p_company, p_plan, p_plan, p_interval, v_budget, v_volume, 'trialing')
  returning id into v_tenant;

  insert into people (tenant_id, email, display_name, role)
  values (v_tenant, p_email, p_display_name, 'admin')
  returning id into v_person;

  insert into audit_log (tenant_id, actor_id, action, subject_type, subject_id)
  values (v_tenant, v_person, 'tenant.provisioned', 'tenant', v_tenant);

  return query select v_tenant, v_person, true;
end;
$$;

revoke all on function public.provision_tenant(text, text, text, text, text) from public;
grant execute on function public.provision_tenant(text, text, text, text, text) to brain_app;

create function public.tenant_set_subscription(
  p_tenant_id uuid,
  p_customer_id text,
  p_subscription_id text,
  p_status text,
  p_plan text,
  p_interval text
) returns void
language plpgsql
security definer
set search_path = public
as $$
declare
  v_budget int;
  v_volume int;
begin
  select pl.entry_budget, pl.included_query_volume into v_budget, v_volume
  from plan_limits pl where pl.plan = p_plan;

  update tenants set
    stripe_customer_id = coalesce(p_customer_id, stripe_customer_id),
    stripe_subscription_id = coalesce(p_subscription_id, stripe_subscription_id),
    subscription_status = p_status,
    plan = coalesce(p_plan, plan),
    tier = coalesce(p_plan, tier),
    billing_interval = coalesce(p_interval, billing_interval),
    entry_budget = coalesce(v_budget, entry_budget),
    included_query_volume = coalesce(v_volume, included_query_volume)
  where id = p_tenant_id;
end;
$$;

revoke all on function public.tenant_set_subscription(uuid, text, text, text, text, text) from public;
grant execute on function public.tenant_set_subscription(uuid, text, text, text, text, text) to brain_app;
