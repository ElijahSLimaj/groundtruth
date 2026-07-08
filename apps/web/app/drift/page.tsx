import Link from 'next/link';

import { HealthRing } from '../../components/health-ring';
import { EmptyState, PageHeader } from '../../components/page-header';
import { withTenant } from '../../lib/db';
import { requireViewer } from '../../lib/session';
import { formatDate } from '../../lib/trust';

export const dynamic = 'force-dynamic';

export default async function DriftPage() {
  const viewer = await requireViewer();

  const data = await withTenant(viewer.tenantId, async (client) => {
    const stats = await client.query<{
      open_contradictions: string;
      pending_proposals: string;
      recurring: string;
    }>(
      `select
         count(*) filter (where kind in ('contradiction', 'extension') and status = 'pending') as open_contradictions,
         count(*) filter (where status in ('pending', 'queued')) as pending_proposals,
         count(*) filter (where recurring_after_rejection and status = 'pending') as recurring
       from drift_proposals`,
    );
    const health = await client.query<{
      domain: string;
      score: string;
      open_contradictions: string;
    }>(
      `select domain, score::text, open_contradictions::text from public.canon_health()`,
    );
    const conflicts = await client.query<{
      id: string;
      drafted_statement: string;
      current_statement: string | null;
      domain: string | null;
      confidence: string;
      created_at: Date;
      strategic: boolean;
    }>(
      `select dp.id, dp.drafted_statement, cv.statement as current_statement,
              coalesce(ce.domain, dp.domain) as domain, dp.confidence::text,
              dp.created_at, dp.strategic
       from drift_proposals dp
       left join canon_entries ce on ce.id = dp.entry_id
       left join canon_versions cv on cv.id = ce.current_version_id
       where dp.kind in ('contradiction', 'extension') and dp.status = 'pending'
       order by dp.strategic desc, dp.confidence desc
       limit 6`,
    );
    return {
      stats: stats.rows[0],
      health: health.rows,
      conflicts: conflicts.rows,
    };
  });

  const avgHealth =
    data.health.length > 0
      ? data.health.reduce((sum, h) => sum + Number(h.score), 0) /
        data.health.length
      : 0;

  const stat = (value: string | number, label: string, tone = 'text-ink') => (
    <div className="rounded-card border border-line bg-surface px-6 py-5">
      <p className={`font-display font-extrabold text-2xl ${tone}`}>{value}</p>
      <p className="mt-1 eyebrow text-ink-muted">{label}</p>
    </div>
  );

  return (
    <div className="max-w-5xl">
      <PageHeader
        title="Drift"
        subtitle="Where the stream disagrees with the canon."
      />

      <div className="grid grid-cols-3 gap-4">
        {stat(
          Number(data.stats.open_contradictions),
          'open contradictions',
          Number(data.stats.open_contradictions) > 0
            ? 'text-conflict'
            : 'text-ink',
        )}
        {stat(Number(data.stats.pending_proposals), 'pending proposals')}
        {stat(Math.round(avgHealth), 'canon health', 'text-verified')}
      </div>

      <section className="mt-8">
        <h2 className="eyebrow text-ink-secondary mb-3">domains</h2>
        {data.health.length === 0 ? (
          <EmptyState
            title="No canon yet"
            detail="Run cold start or approve your first entries to see domain health."
          />
        ) : (
          <div className="grid grid-cols-3 gap-4">
            {data.health.map((h) => (
              <Link
                key={h.domain}
                href={`/canon?domain=${h.domain}`}
                className="flex items-center gap-4 rounded-card border border-line bg-surface px-5 py-4 hover:bg-raised"
              >
                <HealthRing score={Number(h.score)} />
                <div>
                  <p className="text-sm text-ink">{h.domain}</p>
                  <p className="eyebrow text-ink-muted">
                    {h.open_contradictions} open
                  </p>
                </div>
              </Link>
            ))}
          </div>
        )}
      </section>

      <section className="mt-8">
        <h2 className="eyebrow text-ink-secondary mb-3">open contradictions</h2>
        {data.conflicts.length === 0 ? (
          <p className="text-sm text-ink-muted">
            Nothing contradicts the canon right now.
          </p>
        ) : (
          <ul className="flex flex-col gap-3">
            {data.conflicts.map((conflict) => (
              <li
                key={conflict.id}
                className="rounded-card border-l-2 border-conflict bg-surface px-5 py-4"
              >
                <div className="grid grid-cols-2 gap-6">
                  <div>
                    <p className="eyebrow text-ink-muted mb-1">canon says</p>
                    <p className="text-sm text-ink-secondary">
                      {conflict.current_statement ?? 'no entry yet'}
                    </p>
                  </div>
                  <div>
                    <p className="eyebrow text-stream mb-1">stream says</p>
                    <p className="text-sm text-ink">
                      {conflict.drafted_statement}
                    </p>
                  </div>
                </div>
                <p className="mt-3 font-mono text-xs text-ink-muted">
                  {conflict.domain ?? 'unclassified'} ·{' '}
                  {Math.round(Number(conflict.confidence) * 100)}% ·{' '}
                  {formatDate(conflict.created_at)}
                  {conflict.strategic ? ' · strategic' : ''}
                </p>
              </li>
            ))}
          </ul>
        )}
        <p className="mt-4 text-sm">
          <Link href="/queue" className="text-action hover:underline">
            Review in the queue
          </Link>
        </p>
      </section>
    </div>
  );
}
