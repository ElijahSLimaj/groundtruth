begin;
create extension if not exists pgtap with schema extensions;
set local search_path = public, extensions;

select plan(7);

select set_config('role', 'postgres', true);

select lives_ok(
  $$select public.provision_tenant('founder@newco.test', 'Ada Founder', 'NewCo', 'growth', 'year')$$,
  'provision_tenant runs for a brand new email'
);

select is(
  (select plan from tenants where name = 'NewCo'),
  'growth',
  'tenant is created on the requested plan'
);

select is(
  (select entry_budget from tenants where name = 'NewCo'),
  400,
  'entry budget is taken from plan_limits, not the default'
);

select is(
  (select role from people where lower(email) = 'founder@newco.test'),
  'admin',
  'the first person is provisioned as admin'
);

select is(
  (select subscription_status from tenants where name = 'NewCo'),
  'trialing',
  'a freshly provisioned tenant starts trialing'
);

select is(
  (select created from public.provision_tenant('founder@newco.test', 'Ada', 'NewCo', 'growth', 'year')),
  false,
  'provisioning is idempotent, an existing email is not recreated'
);

select is(
  (select count(*) from tenants where name = 'NewCo'),
  1::bigint,
  'idempotent provisioning never duplicates the tenant'
);

select * from finish();
rollback;
