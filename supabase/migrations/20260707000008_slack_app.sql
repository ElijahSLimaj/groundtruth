alter table people add column slack_user_id text;
alter table drift_proposals add column slack_notified_at timestamptz;

create index people_slack_user_idx on people (tenant_id, slack_user_id)
  where slack_user_id is not null;
