begin;
create extension if not exists pgtap with schema extensions;
set local search_path = public, extensions;

select plan(9);

select ok(
  public.acl_admits('{"scope": "tenant"}'::jsonb, '{}'),
  'tenant scope admits a caller holding no principals'
);

select ok(
  public.acl_admits('{"scope": "tenant"}'::jsonb, array['person:a']),
  'tenant scope admits any tenant member'
);

select ok(
  public.acl_admits(
    '{"scope": "principals", "principals": ["person:a", "person:b"]}'::jsonb,
    array['person:b']
  ),
  'principals scope admits a listed principal'
);

select ok(
  not public.acl_admits(
    '{"scope": "principals", "principals": ["person:a"]}'::jsonb,
    array['person:b']
  ),
  'principals scope denies an unlisted principal'
);

select ok(
  not public.acl_admits(
    '{"scope": "principals", "principals": ["person:a"]}'::jsonb,
    '{}'
  ),
  'an empty principal set is admitted by nothing'
);

select ok(
  not public.acl_admits(
    '{"scope": "principals", "principals": ["person:a"]}'::jsonb,
    null
  ),
  'a null principal set is admitted by nothing'
);

select ok(
  not public.acl_admits(null, array['person:a']),
  'a null acl admits nobody'
);

select ok(
  not public.acl_admits('{"scope": "group", "group": "eng"}'::jsonb, array['person:a']),
  'an unrecognised scope denies by default rather than falling open'
);

select ok(
  not public.acl_admits('{}'::jsonb, array['person:a']),
  'an acl with no scope denies by default'
);

select * from finish();
rollback;
