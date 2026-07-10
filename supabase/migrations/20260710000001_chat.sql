create table chat_conversations (
  id uuid primary key default gen_random_uuid(),
  tenant_id uuid not null references tenants(id),
  person_id uuid not null references people(id),
  title text not null default 'New conversation',
  created_at timestamptz not null default now(),
  updated_at timestamptz not null default now()
);

create index chat_conversations_person_idx
  on chat_conversations (tenant_id, person_id, updated_at desc);

create table chat_messages (
  id uuid primary key default gen_random_uuid(),
  tenant_id uuid not null references tenants(id),
  conversation_id uuid not null references chat_conversations(id),
  role text not null check (role in ('user', 'assistant')),
  content text not null,
  citations jsonb not null default '[]',
  created_at timestamptz not null default now()
);

create index chat_messages_conversation_idx
  on chat_messages (conversation_id, created_at);

create table chat_artifacts (
  id uuid primary key default gen_random_uuid(),
  tenant_id uuid not null references tenants(id),
  conversation_id uuid not null references chat_conversations(id),
  message_id uuid references chat_messages(id),
  kind text not null check (kind in ('document', 'deck', 'image', 'video')),
  title text not null,
  content jsonb not null,
  created_at timestamptz not null default now()
);

create index chat_artifacts_conversation_idx
  on chat_artifacts (conversation_id, created_at);

grant select, insert, update, delete on chat_conversations, chat_messages, chat_artifacts to brain_app;

alter table chat_conversations enable row level security;
alter table chat_messages enable row level security;
alter table chat_artifacts enable row level security;

create policy tenant_isolation on chat_conversations
  for all to brain_app
  using (tenant_id = public.app_tenant_id())
  with check (tenant_id = public.app_tenant_id());

create policy tenant_isolation on chat_messages
  for all to brain_app
  using (tenant_id = public.app_tenant_id())
  with check (tenant_id = public.app_tenant_id());

create policy tenant_isolation on chat_artifacts
  for all to brain_app
  using (tenant_id = public.app_tenant_id())
  with check (tenant_id = public.app_tenant_id());
