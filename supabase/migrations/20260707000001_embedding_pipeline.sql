alter table events add column author_source_ref text;

alter table event_chunks add column token_count int not null;
alter table event_chunks add column window_key text not null;
alter table event_chunks add column member_event_ids uuid[] not null;

create index event_chunks_window_idx on event_chunks (tenant_id, embedding_model, window_key);
create index event_chunks_members_idx on event_chunks using gin (member_event_ids);

create table embedding_watermark (
  embedding_model text primary key,
  last_ingested_at timestamptz not null,
  last_event_id uuid not null default '00000000-0000-0000-0000-000000000000',
  updated_at timestamptz not null default now()
);

create table embedding_dlq (
  id bigint generated always as identity primary key,
  tenant_id uuid not null references tenants(id),
  event_id uuid,
  chunk_index int,
  reason text not null,
  failed_at timestamptz not null default now()
);

create index embedding_dlq_tenant_idx on embedding_dlq (tenant_id);
create index events_ingested_idx on events (ingested_at, id);

do $$
begin
  if not exists (select from pg_roles where rolname = 'brain_embedder') then
    create role brain_embedder nologin;
  end if;
end;
$$;

grant brain_embedder to postgres;

grant usage on schema public to brain_embedder;
grant usage on schema extensions to brain_embedder;
grant usage on all sequences in schema public to brain_embedder;

grant select on events to brain_embedder;
grant select, insert, delete on event_chunks to brain_embedder;
grant select, insert, update on embedding_watermark to brain_embedder;
grant select, insert on embedding_dlq to brain_embedder;

alter table embedding_watermark enable row level security;
alter table embedding_dlq enable row level security;

create policy embedder_read_all on events
  for select to brain_embedder
  using (true);

create policy embedder_all_chunks_select on event_chunks
  for select to brain_embedder
  using (true);

create policy embedder_all_chunks_insert on event_chunks
  for insert to brain_embedder
  with check (true);

create policy embedder_all_chunks_delete on event_chunks
  for delete to brain_embedder
  using (true);

create policy embedder_watermark on embedding_watermark
  for all to brain_embedder
  using (true)
  with check (true);

create policy embedder_dlq on embedding_dlq
  for all to brain_embedder
  using (true)
  with check (true);
