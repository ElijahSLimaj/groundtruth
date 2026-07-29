alter table metering_events add column category text not null default 'tool_call'
  check (category in ('tool_call', 'agent_run', 'system'));
alter table metering_events add column model text;
alter table metering_events add column input_tokens int not null default 0 check (input_tokens >= 0);
alter table metering_events add column output_tokens int not null default 0 check (output_tokens >= 0);
alter table metering_events add column billable boolean not null default true;

create index metering_events_billable_day_idx
  on metering_events (tenant_id, occurred_at)
  where billable;

create view metering_cost_daily
with (security_invoker = true) as
select tenant_id,
       date_trunc('day', occurred_at) as day,
       category,
       count(*) as runs,
       sum(input_tokens) as input_tokens,
       sum(output_tokens) as output_tokens
from metering_events
where billable
group by tenant_id, date_trunc('day', occurred_at), category;

grant select on metering_cost_daily to brain_app;
