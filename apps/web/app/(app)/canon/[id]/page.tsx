import { notFound } from 'next/navigation';

import { PageHeader } from '../../../../components/page-header';
import { Receipt } from '../../../../components/receipt';
import { TrustBadge } from '../../../../components/trust-badge';
import { withTenant } from '../../../../lib/db';
import { requireViewer } from '../../../../lib/session';
import { entryTrust, formatDate } from '../../../../lib/trust';

export const dynamic = 'force-dynamic';

export default async function EntryPage(props: {
  params: Promise<{ id: string }>;
}) {
  const { id } = await props.params;
  const viewer = await requireViewer();

  const data = await withTenant(viewer.tenantId, async (client) => {
    const entry = await client.query<{
      id: string;
      domain: string;
      tier: string;
      status: string;
      verified_at: Date | null;
      verify_interval: string;
      statement: string | null;
      attributes: Record<string, unknown> | null;
      owner_name: string;
    }>(
      `select ce.id, ce.domain, ce.tier, ce.status, ce.verified_at,
              ce.verify_interval::text as verify_interval,
              cv.statement, cv.attributes, p.display_name as owner_name
       from canon_entries ce
       left join canon_versions cv on cv.id = ce.current_version_id
       join people p on p.id = ce.owner_id
       where ce.id = $1`,
      [id],
    );
    if (!entry.rows[0]) {
      return null;
    }
    const versions = await client.query<{
      id: string;
      version_number: number;
      statement: string;
      status: string;
      created_at: Date;
      author_name: string;
      approver_name: string | null;
    }>(
      `select cv.id, cv.version_number, cv.statement, cv.status, cv.created_at,
              p.display_name as author_name, ap_p.display_name as approver_name
       from canon_versions cv
       join people p on p.id = cv.created_by
       left join lateral (
         select approver_id from approvals a
         where a.version_id = cv.id and a.decision = 'approved'
         order by a.decided_at desc limit 1
       ) ap on true
       left join people ap_p on ap_p.id = ap.approver_id
       where cv.entry_id = $1
       order by cv.version_number desc`,
      [id],
    );
    const provenance = await client.query<{ n: string }>(
      `select count(*) as n from canon_provenance cp
       join canon_versions cv on cv.id = cp.version_id
       where cv.entry_id = $1`,
      [id],
    );
    const relations = await client.query<{
      relation: string;
      other: string;
    }>(
      `select cr.relation,
              case when cr.from_entry = $1 then cr.to_entry else cr.from_entry end as other
       from canon_relations cr
       where cr.from_entry = $1 or cr.to_entry = $1`,
      [id],
    );
    return {
      entry: entry.rows[0],
      versions: versions.rows,
      sourceCount: Number(provenance.rows[0].n),
      relations: relations.rows,
    };
  });

  if (!data) {
    notFound();
  }
  const { entry, versions, sourceCount, relations } = data;
  const approver =
    versions.find((v) => v.status === 'approved')?.approver_name ?? null;

  return (
    <div className="max-w-3xl">
      <PageHeader title={entry.domain} subtitle={`${entry.tier} · owned by ${entry.owner_name}`} />

      <article className="rounded-card border border-line bg-surface p-6 shadow-panel">
        <div className="flex items-start justify-between gap-4">
          <p className="text-lg leading-relaxed">
            {entry.statement ?? 'No approved version yet.'}
          </p>
          <TrustBadge label={entryTrust(entry.status)} />
        </div>
        <div className="mt-4">
          <Receipt
            receipt={{
              approver,
              verifiedAt: entry.verified_at,
              sourceCount,
            }}
          />
        </div>
        {entry.attributes && Object.keys(entry.attributes).length > 0 ? (
          <dl className="mt-5 rounded-control border border-line bg-void p-3 font-mono text-xs flex flex-col gap-1">
            {Object.entries(entry.attributes).map(([key, value]) => (
              <div key={key} className="flex gap-3">
                <dt className="min-w-48 text-ink-muted">{key}</dt>
                <dd className="text-ink-secondary">{JSON.stringify(value)}</dd>
              </div>
            ))}
          </dl>
        ) : null}
      </article>

      <section className="mt-8">
        <h2 className="eyebrow text-ink-secondary mb-3">version history</h2>
        <ol className="relative flex flex-col gap-4 border-l border-verified/40 pl-5">
          {versions.map((version) => (
            <li key={version.id}>
              <span
                aria-hidden
                className="absolute -left-[5px] mt-1.5 h-2 w-2 rounded-full bg-verified"
              />
              <p className="text-sm text-ink">{version.statement}</p>
              <p className="mt-0.5 font-mono text-xs text-ink-muted">
                v{version.version_number} · {version.status} ·{' '}
                {formatDate(version.created_at)} · by {version.author_name}
                {version.approver_name
                  ? ` · approved by ${version.approver_name}`
                  : ''}
              </p>
            </li>
          ))}
        </ol>
      </section>

      {relations.length > 0 ? (
        <section className="mt-8">
          <h2 className="eyebrow text-ink-secondary mb-3">relations</h2>
          <ul className="flex flex-wrap gap-2">
            {relations.map((relation, index) => (
              <li
                key={index}
                className="rounded-full border border-line px-3 py-1 font-mono text-xs text-ink-secondary"
              >
                {relation.relation} · {relation.other.slice(0, 8)}
              </li>
            ))}
          </ul>
        </section>
      ) : null}
    </div>
  );
}
