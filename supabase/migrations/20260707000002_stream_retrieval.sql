alter table event_chunks add column source_type text not null;
alter table event_chunks add column tombstoned boolean not null default false;

create index event_chunks_fts_idx on event_chunks
  using gin (to_tsvector('english', content));

create index event_chunks_tenant_visible_hnsw_idx on event_chunks
  using hnsw (embedding extensions.vector_cosine_ops)
  where ((acl->>'scope') = 'tenant');

create table retrieval_half_life (
  source_type text primary key,
  half_life interval not null
);

insert into retrieval_half_life (source_type, half_life) values
  ('slack', interval '7 days'),
  ('gmail', interval '30 days'),
  ('drive', interval '90 days'),
  ('notion', interval '90 days'),
  ('transcript', interval '3 days');

grant select on retrieval_half_life to brain_app;
grant select on retrieval_half_life to brain_embedder;

alter table retrieval_half_life enable row level security;

create policy read_all on retrieval_half_life
  for select to brain_app
  using (true);

create policy embedder_read_all on retrieval_half_life
  for select to brain_embedder
  using (true);

create function public.stream_search(
  p_query_embedding extensions.vector(1536),
  p_query_text text,
  p_model text,
  p_principals text[] default '{}',
  p_k int default 12,
  p_source_types text[] default null
) returns table (
  chunk_id uuid,
  chunk_event_id uuid,
  chunk_content text,
  chunk_source_type text,
  chunk_window_key text,
  chunk_occurred_at timestamptz,
  chunk_acl jsonb,
  similarity double precision,
  score double precision
)
language plpgsql
stable
set search_path = public, extensions
as $$
declare
  v_ef int := 40;
  v_fetch int;
  v_iter int := 1;
  v_candidates int;
begin
  v_fetch := greatest(p_k * 4, 20);
  loop
    perform set_config('hnsw.ef_search', v_ef::text, true);
    select count(*) into v_candidates from (
      (select c.id
       from event_chunks c
       where c.embedding_model = p_model
         and not c.tombstoned
         and c.acl->>'scope' = 'tenant'
         and (p_source_types is null or c.source_type = any(p_source_types))
       order by c.embedding <=> p_query_embedding
       limit v_fetch)
      union all
      (select c.id
       from event_chunks c
       where c.embedding_model = p_model
         and not c.tombstoned
         and c.acl->>'scope' = 'principals'
         and c.acl->'principals' ?| p_principals
         and (p_source_types is null or c.source_type = any(p_source_types))
       order by c.embedding <=> p_query_embedding
       limit v_fetch)
    ) s;
    exit when v_candidates >= p_k or v_iter >= 3;
    v_iter := v_iter + 1;
    v_ef := v_ef * 2;
    v_fetch := v_fetch * 2;
  end loop;

  return query
  with vector_candidates as (
    select u.cid, min(u.dist) as dist
    from (
      (select c.id as cid, c.embedding <=> p_query_embedding as dist
       from event_chunks c
       where c.embedding_model = p_model
         and not c.tombstoned
         and c.acl->>'scope' = 'tenant'
         and (p_source_types is null or c.source_type = any(p_source_types))
       order by c.embedding <=> p_query_embedding
       limit v_fetch)
      union all
      (select c.id, c.embedding <=> p_query_embedding
       from event_chunks c
       where c.embedding_model = p_model
         and not c.tombstoned
         and c.acl->>'scope' = 'principals'
         and c.acl->'principals' ?| p_principals
         and (p_source_types is null or c.source_type = any(p_source_types))
       order by c.embedding <=> p_query_embedding
       limit v_fetch)
    ) u
    group by u.cid
  ),
  vector_ranked as (
    select vc.cid, vc.dist, row_number() over (order by vc.dist) as rnk
    from vector_candidates vc
  ),
  text_ranked as (
    select c.id as cid,
           row_number() over (order by ts_rank_cd(to_tsvector('english', c.content), q) desc) as rnk
    from event_chunks c
    cross join websearch_to_tsquery('english', p_query_text) q
    where c.embedding_model = p_model
      and not c.tombstoned
      and to_tsvector('english', c.content) @@ q
      and (c.acl->>'scope' = 'tenant'
           or (c.acl->>'scope' = 'principals' and c.acl->'principals' ?| p_principals))
      and (p_source_types is null or c.source_type = any(p_source_types))
    limit v_fetch
  ),
  fused as (
    select coalesce(v.cid, t.cid) as cid,
           coalesce(1.0 / (60 + v.rnk), 0) + coalesce(1.0 / (60 + t.rnk), 0) as rrf,
           v.dist
    from vector_ranked v
    full outer join text_ranked t on v.cid = t.cid
  )
  select c.id,
         c.event_id,
         c.content,
         c.source_type,
         c.window_key,
         c.event_occurred_at,
         c.acl,
         (1 - coalesce(f.dist, 1))::double precision,
         (f.rrf * exp(
            -ln(2) * greatest(extract(epoch from (now() - c.event_occurred_at)), 0)
            / extract(epoch from coalesce(h.half_life, interval '30 days'))
         ))::double precision
  from fused f
  join event_chunks c on c.id = f.cid
  left join retrieval_half_life h on h.source_type = c.source_type
  order by 9 desc
  limit p_k;
end;
$$;

grant execute on function public.stream_search to brain_app;
