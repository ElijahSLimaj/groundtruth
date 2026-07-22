delete from embedding_dlq d
where exists (
  select 1 from embedding_dlq keep
  where keep.tenant_id = d.tenant_id
    and keep.event_id is not distinct from d.event_id
    and keep.chunk_index is not distinct from d.chunk_index
    and keep.reason = d.reason
    and keep.failed_at < d.failed_at
);

create unique index embedding_dlq_unique_failure
  on embedding_dlq (tenant_id, event_id, chunk_index, reason)
  nulls not distinct;
