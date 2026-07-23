select public.create_monthly_partitions('events', date '2026-07-01', 0);

insert into tenants (id, name, tier) values
  ('11111111-1111-1111-1111-111111111111', 'Acme Robotics', 'growth')
on conflict (id) do nothing;

insert into people (id, tenant_id, email, display_name, role) values
  ('22222222-0000-0000-0000-000000000001', '11111111-1111-1111-1111-111111111111', 'founder@acme.test', 'Ada Founder', 'admin'),
  ('22222222-0000-0000-0000-000000000002', '11111111-1111-1111-1111-111111111111', 'sales@acme.test', 'Sam Sales', 'owner'),
  ('22222222-0000-0000-0000-000000000003', '11111111-1111-1111-1111-111111111111', 'eng@acme.test', 'Eve Engineer', 'member'),
  ('22222222-0000-0000-0000-000000000004', '11111111-1111-1111-1111-111111111111', 'support-agent@acme.test', 'Support Agent', 'agent'),
  ('22222222-0000-0000-0000-000000000005', '11111111-1111-1111-1111-111111111111', 'elijah@buildstackhq.com', 'Elijah Limaj', 'admin')
on conflict (id) do nothing;

insert into connectors (id, tenant_id, source_type, status, config) values
  ('33333333-0000-0000-0000-000000000001', '11111111-1111-1111-1111-111111111111', 'slack', 'live', '{"workspace": "acme.slack.test"}'),
  ('33333333-0000-0000-0000-000000000002', '11111111-1111-1111-1111-111111111111', 'gmail', 'live', '{"domain": "acme.test"}')
on conflict (id) do nothing;

insert into events (id, tenant_id, connector_id, source_type, external_id, author_id, thread_key, occurred_at, acl, payload_ref) values
  ('44444444-0000-0000-0000-000000000001', '11111111-1111-1111-1111-111111111111', '33333333-0000-0000-0000-000000000001', 'slack', '1751713200.000100', '22222222-0000-0000-0000-000000000002', '1751713200.000100', timestamptz '2026-07-03T09:15:00Z', '{"scope": "tenant", "source_scope": {"type": "slack_channel", "id": "C0SALES", "visibility": "public"}}', 'payloads/11111111/slack/1751713200.000100'),
  ('44444444-0000-0000-0000-000000000002', '11111111-1111-1111-1111-111111111111', '33333333-0000-0000-0000-000000000001', 'slack', '1751799600.000200', '22222222-0000-0000-0000-000000000001', '1751799600.000200', timestamptz '2026-07-04T14:30:00Z', '{"scope": "principals", "principals": ["person:22222222-0000-0000-0000-000000000001", "person:22222222-0000-0000-0000-000000000002"], "source_scope": {"type": "slack_channel", "id": "C0FOUNDERS", "visibility": "private"}}', 'payloads/11111111/slack/1751799600.000200'),
  ('44444444-0000-0000-0000-000000000003', '11111111-1111-1111-1111-111111111111', '33333333-0000-0000-0000-000000000002', 'gmail', 'gmail-thread-8842-msg-1', '22222222-0000-0000-0000-000000000002', 'gmail-thread-8842', timestamptz '2026-07-05T08:45:00Z', '{"scope": "principals", "principals": ["person:22222222-0000-0000-0000-000000000001", "person:22222222-0000-0000-0000-000000000002"], "source_scope": {"type": "gmail_thread", "id": "gmail-thread-8842"}}', 'payloads/11111111/gmail/gmail-thread-8842-msg-1')
on conflict (tenant_id, connector_id, external_id, occurred_at) do nothing;

insert into canon_entries (id, tenant_id, domain, tier, owner_id, status, visibility, verify_interval) values
  ('55555555-0000-0000-0000-000000000001', '11111111-1111-1111-1111-111111111111', 'pricing', 'bedrock', '22222222-0000-0000-0000-000000000002', 'active', '{"scope": "tenant"}', interval '90 days'),
  ('55555555-0000-0000-0000-000000000002', '11111111-1111-1111-1111-111111111111', 'product', 'operational', '22222222-0000-0000-0000-000000000003', 'active', '{"scope": "tenant"}', interval '30 days')
on conflict (id) do nothing;

insert into canon_versions (id, tenant_id, entry_id, version_number, statement, created_by, status, attributes) values
  ('66666666-0000-0000-0000-000000000001', '11111111-1111-1111-1111-111111111111', '55555555-0000-0000-0000-000000000001', 1, 'Starter plan is 499 per month, Growth is 1499 per month, annual billing gets two months free. Discounts above 15 percent require founder approval.', '22222222-0000-0000-0000-000000000002', 'approved', '{"plan": "growth", "amount": 1499, "currency": "USD", "billing_period": "monthly", "discount_ceiling_percent": 15}'),
  ('66666666-0000-0000-0000-000000000002', '11111111-1111-1111-1111-111111111111', '55555555-0000-0000-0000-000000000002', 1, 'The RX-2 arm ships with the vision module included as of the June release. The standalone vision SKU is discontinued.', '22222222-0000-0000-0000-000000000003', 'approved', '{}')
on conflict (id) do nothing;

update canon_entries set current_version_id = '66666666-0000-0000-0000-000000000001', verified_at = now()
  where id = '55555555-0000-0000-0000-000000000001' and current_version_id is null;
update canon_entries set current_version_id = '66666666-0000-0000-0000-000000000002', verified_at = now()
  where id = '55555555-0000-0000-0000-000000000002' and current_version_id is null;

insert into canon_provenance (tenant_id, version_id, event_id, event_occurred_at)
select '11111111-1111-1111-1111-111111111111', '66666666-0000-0000-0000-000000000001', id, occurred_at
  from events where id = '44444444-0000-0000-0000-000000000001'
on conflict (version_id, event_id) do nothing;

insert into approval_policies (id, tenant_id, tier, domain, required_role, required_approver_count) values
  ('77777777-0000-0000-0000-000000000001', '11111111-1111-1111-1111-111111111111', 'bedrock', null, 'admin', 1),
  ('77777777-0000-0000-0000-000000000002', '11111111-1111-1111-1111-111111111111', 'operational', null, 'owner', 1)
on conflict (id) do nothing;

insert into approvals (id, tenant_id, version_id, approver_id, decision, note) values
  ('88888888-0000-0000-0000-000000000001', '11111111-1111-1111-1111-111111111111', '66666666-0000-0000-0000-000000000001', '22222222-0000-0000-0000-000000000001', 'approved', 'Matches the June pricing decision'),
  ('88888888-0000-0000-0000-000000000002', '11111111-1111-1111-1111-111111111111', '66666666-0000-0000-0000-000000000002', '22222222-0000-0000-0000-000000000003', 'approved', null)
on conflict (id) do nothing;

insert into drift_proposals (id, tenant_id, entry_id, kind, drafted_statement, confidence, routed_to) values
  ('99999999-0000-0000-0000-000000000001', '11111111-1111-1111-1111-111111111111', '55555555-0000-0000-0000-000000000001', 'contradiction', 'Growth plan pricing changed to 1799 per month effective August, per the founder announcement in the sales channel.', 0.87, '22222222-0000-0000-0000-000000000002')
on conflict (id) do nothing;
