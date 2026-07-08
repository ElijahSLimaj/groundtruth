import Link from 'next/link';

import { HealthRing } from '../../components/health-ring';
import { EmptyState, PageHeader } from '../../components/page-header';
import { TrustBadge } from '../../components/trust-badge';
import { withTenant } from '../../lib/db';
import { requireViewer } from '../../lib/session';
import { entryTrust, formatDate } from '../../lib/trust';

export const dynamic = 'force-dynamic';

interface EntryRow {
  id: string;
  domain: string;
  tier: string;
  status: string;
  verified_at: Date | null;
  statement: string | null;
  owner_name: string;
}

interface HealthRow {
  domain: string;
  score: string;
}

export default async function CanonPage(props: {
  searchParams: Promise<{ domain?: string }>;
}) {
  const { domain } = await props.searchParams;
  const viewer = await requireViewer();

  const { entries, health } = await withTenant(viewer.tenantId, async (client) => {
    const health = await client.query<HealthRow>(
      `select domain, score::text from public.canon_health()`,
    );
    const entries = await client.query<EntryRow>(
      `select ce.id, ce.domain, ce.tier, ce.status, ce.verified_at,
              cv.statement, p.display_name as owner_name
       from canon_entries ce
       left join canon_versions cv on cv.id = ce.current_version_id
       join people p on p.id = ce.owner_id
       where ce.status <> 'archived' and ($1::text is null or ce.domain = $1)
       order by ce.domain, ce.verified_at desc nulls last
       limit 200`,
      [domain ?? null],
    );
    return { entries: entries.rows, health: health.rows };
  });

  const domains = [...new Set(health.map((h) => h.domain))].sort();

  return (
    <div className="max-w-5xl">
      <PageHeader
        title="Canon"
        subtitle="The company's governed truth, one owner per fact."
      />
      <div className="flex gap-8">
        <aside className="w-48 shrink-0">
          <ul className="flex flex-col gap-1">
            <li>
              <Link
                href="/canon"
                className={`block rounded-control px-3 py-2 text-sm ${!domain ? 'bg-raised text-ink' : 'text-ink-secondary hover:text-ink'}`}
              >
                All domains
              </Link>
            </li>
            {domains.map((d) => {
              const score = health.find((h) => h.domain === d)?.score;
              return (
                <li key={d}>
                  <Link
                    href={`/canon?domain=${d}`}
                    className={`flex items-center justify-between rounded-control px-3 py-2 text-sm ${domain === d ? 'bg-raised text-ink' : 'text-ink-secondary hover:text-ink'}`}
                  >
                    <span>{d}</span>
                    <span className="font-mono text-xs text-ink-muted">
                      {score ? Math.round(Number(score)) : '—'}
                    </span>
                  </Link>
                </li>
              );
            })}
          </ul>
        </aside>

        <section className="min-w-0 flex-1">
          {entries.length === 0 ? (
            <EmptyState
              title="No entries yet"
              detail="Cold start drafts and approved proposals land here."
            />
          ) : (
            <ul className="flex flex-col divide-y divide-line rounded-card border border-line bg-surface">
              {entries.map((entry) => (
                <li key={entry.id}>
                  <Link
                    href={`/canon/${entry.id}`}
                    className="flex items-center gap-4 px-5 py-3 hover:bg-raised focus-visible:outline focus-visible:outline-2 focus-visible:outline-action"
                  >
                    <div className="min-w-0 flex-1">
                      <p className="truncate text-sm text-ink">
                        {entry.statement ?? 'Draft without an approved version'}
                      </p>
                      <p className="mt-0.5 eyebrow text-ink-muted">
                        {entry.domain} · {entry.tier} · {entry.owner_name}
                      </p>
                    </div>
                    <TrustBadge label={entryTrust(entry.status)} />
                    <span className="font-mono text-xs text-ink-muted w-24 text-right">
                      {formatDate(entry.verified_at)}
                    </span>
                  </Link>
                </li>
              ))}
            </ul>
          )}
        </section>
      </div>
      <div className="mt-8 flex gap-6">
        {health.map((h) => (
          <div key={h.domain} className="flex items-center gap-2">
            <HealthRing score={Number(h.score)} />
            <span className="text-xs text-ink-secondary">{h.domain}</span>
          </div>
        ))}
      </div>
    </div>
  );
}
