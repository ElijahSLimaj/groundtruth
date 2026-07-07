import { EmptyState, PageHeader } from '../../components/page-header';
import { withTenant } from '../../lib/db';
import { getViewer } from '../../lib/session';

export const dynamic = 'force-dynamic';

const STATUS_TONE: Record<string, string> = {
  live: 'text-positive',
  degraded: 'text-stale',
  backfilling: 'text-stream',
};

export default async function ConnectorsPage() {
  const viewer = getViewer();

  const connectors = await withTenant(viewer.tenantId, async (client) => {
    const rows = await client.query<{
      id: string;
      source_type: string;
      status: string;
      created_at: Date;
      lag_seconds: string | null;
    }>(
      `select c.id, c.source_type, c.status, c.created_at,
              extract(epoch from (now() - cs.updated_at))::text as lag_seconds
       from connectors c
       left join connector_state cs on cs.connector_id = c.id
       order by c.source_type`,
    );
    return rows.rows;
  });

  return (
    <div className="max-w-4xl">
      <PageHeader
        title="Connectors"
        subtitle="Every source either becomes an event or lands in the dead letter queue."
      />
      {connectors.length === 0 ? (
        <EmptyState
          title="No connectors yet"
          detail="Connect Slack to start filling the stream."
        />
      ) : (
        <div className="grid grid-cols-2 gap-4">
          {connectors.map((connector) => (
            <div
              key={connector.id}
              className="rounded-card border border-line bg-surface px-5 py-4"
            >
              <div className="flex items-center justify-between">
                <p className="text-md text-ink capitalize">
                  {connector.source_type}
                </p>
                <span
                  className={`eyebrow ${STATUS_TONE[connector.status] ?? 'text-ink-muted'}`}
                >
                  {connector.status}
                </span>
              </div>
              <p className="mt-2 font-mono text-xs text-ink-muted">
                {connector.lag_seconds === null
                  ? 'never polled'
                  : `last cursor ${Math.round(Number(connector.lag_seconds))}s ago`}
              </p>
              {connector.status === 'degraded' ? (
                <p className="mt-2 text-xs text-stale">
                  Check the source credentials, polling is paused and resumes
                  from the cursor on recovery.
                </p>
              ) : null}
            </div>
          ))}
        </div>
      )}
    </div>
  );
}
