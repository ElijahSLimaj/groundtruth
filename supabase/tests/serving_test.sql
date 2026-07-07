begin;
create extension if not exists pgtap with schema extensions;
set local search_path = public, extensions;

select plan(6);

insert into tenants (id, name, tier) values
  ('00000000-0000-0000-0000-00000000000a', 'Tenant A', 'growth'),
  ('00000000-0000-0000-0000-00000000000b', 'Tenant B', 'growth');

insert into people (id, tenant_id, email, display_name, role) values
  ('00000000-0000-0000-0001-00000000000a', '00000000-0000-0000-0000-00000000000a', 'agent@a.test', 'Agent A', 'agent'),
  ('00000000-0000-0000-0001-00000000000b', '00000000-0000-0000-0000-00000000000b', 'agent@b.test', 'Agent B', 'agent');

insert into api_keys (tenant_id, person_id, key_hash, name, allowed_domains) values
  ('00000000-0000-0000-0000-00000000000a', '00000000-0000-0000-0001-00000000000a', 'hash-a', 'agent key a', array['pricing']),
  ('00000000-0000-0000-0000-00000000000b', '00000000-0000-0000-0001-00000000000b', 'hash-b', 'agent key b', null);

update api_keys set revoked_at = now() where key_hash = 'hash-b';

set local role brain_app;

select is(
  (select tenant_id from public.api_key_lookup('hash-a')),
  '00000000-0000-0000-0000-00000000000a'::uuid,
  'key lookup resolves the tenant before any tenant context exists'
);

select is(
  (select person_role || ':' || array_to_string(allowed_domains, ',') from public.api_key_lookup('hash-a')),
  'agent:pricing',
  'key lookup carries the principal role and domain scope'
);

select is(
  (select count(*) from public.api_key_lookup('hash-b')),
  0::bigint,
  'revoked keys never resolve'
);

select is(
  (select count(*) from public.api_key_lookup('hash-unknown')),
  0::bigint,
  'unknown keys never resolve'
);

select is(
  (select count(*) from api_keys),
  0::bigint,
  'the app role sees no keys without tenant context'
);

select set_config('app.tenant_id', '00000000-0000-0000-0000-00000000000a', true);
select is(
  (select count(*) from api_keys),
  1::bigint,
  'with tenant context the app role sees only its own keys'
);

select * from finish();
rollback;
