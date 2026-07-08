create table unmatched_chunks (
  tenant_id uuid not null references tenants(id),
  chunk_id uuid primary key,
  event_id uuid not null,
  author_key text not null,
  added_at timestamptz not null default now()
);

create index unmatched_chunks_tenant_added_idx on unmatched_chunks (tenant_id, added_at);

grant select, insert, delete on unmatched_chunks to brain_app;

alter table unmatched_chunks enable row level security;

create policy tenant_isolation on unmatched_chunks
  for all to brain_app
  using (tenant_id = public.app_tenant_id())
  with check (tenant_id = public.app_tenant_id());

create function public.gap_clusters(
  p_similarity numeric,
  p_min_size int,
  p_min_authors int,
  p_buffer_days int
) returns table (
  cluster_root uuid,
  chunk_id uuid,
  event_id uuid,
  author_key text,
  content text
)
language sql
stable
set search_path = public, extensions
as $$
  with recursive nodes as (
    select uc.chunk_id, uc.event_id, uc.author_key, c.content, c.embedding, c.embedding_model
    from unmatched_chunks uc
    join event_chunks c on c.id = uc.chunk_id and not c.tombstoned
    where uc.added_at > now() - make_interval(days => p_buffer_days)
  ),
  edges as (
    select a.chunk_id as src, b.chunk_id as dst
    from nodes a
    join nodes b
      on a.chunk_id <> b.chunk_id
     and a.embedding_model = b.embedding_model
    where 1 - (a.embedding <=> b.embedding) >= p_similarity
  ),
  reach (a, b) as (
    select src, dst from edges
    union
    select r.a, e.dst from reach r join edges e on e.src = r.b
  ),
  roots as (
    select n.chunk_id, least(n.chunk_id, min(r.b::text)::uuid) as root
    from nodes n
    join reach r on r.a = n.chunk_id
    group by n.chunk_id
  ),
  qualifying as (
    select ro.root
    from roots ro
    join nodes n on n.chunk_id = ro.chunk_id
    group by ro.root
    having count(*) >= p_min_size and count(distinct n.author_key) >= p_min_authors
  )
  select ro.root, n.chunk_id, n.event_id, n.author_key, n.content
  from roots ro
  join nodes n on n.chunk_id = ro.chunk_id
  join qualifying q on q.root = ro.root
  order by ro.root, n.chunk_id;
$$;

grant execute on function public.gap_clusters to brain_app;
