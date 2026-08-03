import { PageHeader } from '../../../components/page-header';
import { withTenant } from '../../../lib/db';
import { requireViewer } from '../../../lib/session';
import { connectApiKey, disconnectConnector } from './actions';

export const dynamic = 'force-dynamic';

const OAUTH_SOURCES = [
  'slack',
  'gmail',
  'gdrive',
  'outlook',
  'teams',
  'notion',
  'hubspot',
  'linear',
  'salesforce',
];
const APIKEY_SOURCES = [
  { source: 'fathom', needsBaseUrl: false },
  { source: 'odoo', needsBaseUrl: true },
];

const STATUS_TONE: Record<string, string> = {
  live: 'text-positive',
  degraded: 'text-stale',
  backfilling: 'text-stream',
  connecting: 'text-stream',
};

function label(source: string): string {
  return source.charAt(0).toUpperCase() + source.slice(1);
}

export default async function ConnectorsPage({
  searchParams,
}: {
  searchParams: Promise<{ connected?: string; error?: string }>;
}) {
  const viewer = await requireViewer();
  const { connected, error } = await searchParams;

  const connectors = await withTenant(viewer.tenantId, async (client) => {
    const rows = await client.query<{
      id: string;
      source_type: string;
      status: string;
      lag_seconds: string | null;
    }>(
      `select c.id, c.source_type, c.status,
              extract(epoch from (now() - cs.updated_at))::text as lag_seconds
       from connectors c
       left join connector_state cs on cs.connector_id = c.id
       where c.status <> 'archived'
       order by c.source_type`,
    );
    return rows.rows;
  });
  const bySource = new Map(connectors.map((c) => [c.source_type, c]));
  const canManage = viewer.role === 'admin' || viewer.role === 'owner';

  return (
    <div className="max-w-4xl">
      <PageHeader
        title="Connectors"
        subtitle="Connect a source and its history flows into the stream, then cold start drafts your canon."
      />

      {connected ? (
        <p className="mb-4 rounded-card border border-positive/40 bg-positive/10 px-4 py-2 text-sm text-positive">
          {label(connected)} connected.
        </p>
      ) : null}
      {error ? (
        <p className="mb-4 rounded-card border border-conflict/40 bg-conflict/10 px-4 py-2 text-sm text-conflict">
          {error === 'oauth_denied'
            ? 'Authorization was cancelled.'
            : 'Could not connect that source. Try again.'}
        </p>
      ) : null}

      <section>
        <p className="eyebrow text-ink-muted mb-3">Available sources</p>
        <div className="grid grid-cols-3 gap-3">
          {OAUTH_SOURCES.map((source) => {
            const existing = bySource.get(source);
            return (
              <div
                key={source}
                className="rounded-card border border-line bg-surface px-4 py-3"
              >
                <div className="flex items-center justify-between">
                  <p className="text-sm text-ink">{label(source)}</p>
                  {existing ? (
                    <span
                      className={`eyebrow ${STATUS_TONE[existing.status] ?? 'text-ink-muted'}`}
                    >
                      {existing.status}
                    </span>
                  ) : null}
                </div>
                {canManage ? (
                  <a
                    href={`/api/connectors/${source}/authorize`}
                    className="mt-3 inline-block rounded-control bg-action px-3 py-1.5 text-xs font-medium text-void hover:opacity-90"
                  >
                    {existing ? 'Reconnect' : 'Connect'}
                  </a>
                ) : null}
              </div>
            );
          })}
        </div>
      </section>

      {canManage ? (
        <section className="mt-8">
          <p className="eyebrow text-ink-muted mb-3">API key sources</p>
          <div className="grid grid-cols-2 gap-3">
            {APIKEY_SOURCES.map(({ source, needsBaseUrl }) => (
              <form
                key={source}
                action={connectApiKey}
                className="rounded-card border border-line bg-surface px-4 py-3"
              >
                <input type="hidden" name="source" value={source} />
                <p className="text-sm text-ink">{label(source)}</p>
                <input
                  name="api_key"
                  type="password"
                  required
                  placeholder="API key"
                  className="mt-2 w-full rounded-control border border-line bg-void px-2 py-1.5 text-xs"
                />
                {needsBaseUrl ? (
                  <input
                    name="base_url"
                    type="url"
                    required
                    placeholder="https://yourcompany.odoo.com"
                    className="mt-2 w-full rounded-control border border-line bg-void px-2 py-1.5 text-xs"
                  />
                ) : null}
                <button
                  type="submit"
                  className="mt-2 rounded-control border border-line-strong px-3 py-1.5 text-xs text-ink-secondary hover:text-ink"
                >
                  {bySource.get(source) ? 'Update key' : 'Connect'}
                </button>
              </form>
            ))}
          </div>
        </section>
      ) : null}

      {connectors.length > 0 ? (
        <section className="mt-8">
          <p className="eyebrow text-ink-muted mb-3">Connected</p>
          <div className="grid grid-cols-2 gap-4">
            {connectors.map((connector) => (
              <div
                key={connector.id}
                className="rounded-card border border-line bg-surface px-5 py-4"
              >
                <div className="flex items-center justify-between">
                  <p className="text-md text-ink">{label(connector.source_type)}</p>
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
                {canManage ? (
                  <form action={disconnectConnector} className="mt-3">
                    <input type="hidden" name="id" value={connector.id} />
                    <button
                      type="submit"
                      className="text-xs text-ink-muted underline hover:text-conflict"
                    >
                      Disconnect
                    </button>
                  </form>
                ) : null}
              </div>
            ))}
          </div>
        </section>
      ) : null}
    </div>
  );
}
