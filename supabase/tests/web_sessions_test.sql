begin;
create extension if not exists pgtap with schema extensions;
set local search_path = public, extensions;

select plan(8);

insert into tenants (id, name, tier) values
  ('00000000-0000-0000-0000-0000000000dd', 'Sessions Tenant', 'growth'),
  ('00000000-0000-0000-0000-0000000000de', 'Other Tenant', 'growth');

insert into people (id, tenant_id, email, display_name, role) values
  ('00000000-0000-0000-0001-0000000000dd', '00000000-0000-0000-0000-0000000000dd', 'founder@sess.test', 'Founder', 'admin');

set local role brain_app;

select is(
  (select person_id from public.web_person_by_email('FOUNDER@sess.test')),
  '00000000-0000-0000-0001-0000000000dd'::uuid,
  'person lookup by email is case insensitive and needs no tenant context'
);

select is(
  (select count(*) from public.web_person_by_email('nobody@sess.test')),
  0::bigint,
  'unknown emails resolve to nothing'
);

select set_config('app.tenant_id', '00000000-0000-0000-0000-0000000000dd', true);

select lives_ok(
  $$insert into web_sessions (tenant_id, person_id, token_hash, expires_at)
    values ('00000000-0000-0000-0000-0000000000dd', '00000000-0000-0000-0001-0000000000dd', 'livehash', now() + interval '12 hours')$$,
  'the app creates sessions inside its tenant'
);

select lives_ok(
  $$insert into web_sessions (tenant_id, person_id, token_hash, expires_at)
    values ('00000000-0000-0000-0000-0000000000dd', '00000000-0000-0000-0001-0000000000dd', 'expiredhash', now() - interval '1 hour')$$,
  'expired fixture session inserts'
);

select set_config('app.tenant_id', null, true);

select is(
  (select display_name from public.web_session_lookup('livehash')),
  'Founder',
  'session lookup resolves the viewer without tenant context'
);

select is(
  (select count(*) from public.web_session_lookup('expiredhash')),
  0::bigint,
  'expired sessions never resolve'
);

select is(
  public.web_session_destroy('livehash'),
  1,
  'destroy removes the session'
);

select is(
  (select count(*) from public.web_session_lookup('livehash')),
  0::bigint,
  'destroyed sessions never resolve'
);

select * from finish();
rollback;
