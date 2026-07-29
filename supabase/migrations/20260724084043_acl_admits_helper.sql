create function public.acl_admits(p_acl jsonb, p_principals text[])
returns boolean
language sql
immutable
set search_path = public, extensions
as $$
  select coalesce(
    p_acl->>'scope' = 'tenant'
    or (
      p_acl->>'scope' = 'principals'
      and p_principals is not null
      and array_length(p_principals, 1) > 0
      and p_acl->'principals' ?| p_principals
    ),
    false
  )
$$;

grant execute on function public.acl_admits(jsonb, text[]) to brain_app;
