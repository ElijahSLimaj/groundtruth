import { EmptyState, PageHeader } from '../../components/page-header';
import { withTenant } from '../../lib/db';
import { requireViewer } from '../../lib/session';

export const dynamic = 'force-dynamic';

export default async function AuditPage(props: {
  searchParams: Promise<{ action?: string }>;
}) {
  const { action } = await props.searchParams;
  const viewer = await requireViewer();

  const rows = await withTenant(viewer.tenantId, async (client) => {
    const result = await client.query<{
      id: string;
      action: string;
      subject_type: string;
      subject_id: string | null;
      occurred_at: Date;
      actor_name: string | null;
      detail: Record<string, unknown>;
    }>(
      `select al.id, al.action, al.subject_type, al.subject_id, al.occurred_at,
              p.display_name as actor_name, al.detail
       from audit_log al
       left join people p on p.id = al.actor_id
       where ($1::text is null or al.action like $1 || '%')
       order by al.occurred_at desc
       limit 100`,
      [action ?? null],
    );
    return result.rows;
  });

  return (
    <div className="max-w-5xl">
      <PageHeader
        title="Audit"
        subtitle="Every state change and every served answer."
      />
      <form className="mb-4 flex gap-2" action="/audit" method="get">
        <label htmlFor="action" className="sr-only">
          Filter by action prefix
        </label>
        <input
          id="action"
          name="action"
          type="text"
          defaultValue={action ?? ''}
          placeholder="Filter by action prefix, e.g. canon."
          className="w-72 rounded-control border border-line bg-surface px-3 py-1.5 font-mono text-xs"
        />
        <button
          type="submit"
          className="rounded-control border border-line-strong px-3 py-1.5 text-xs text-ink-secondary hover:text-ink"
        >
          Filter
        </button>
      </form>

      {rows.length === 0 ? (
        <EmptyState
          title="No audit rows match"
          detail="Loosen the filter or take an action somewhere."
        />
      ) : (
        <div className="overflow-x-auto rounded-card border border-line">
          <table className="w-full bg-surface font-mono text-xs">
            <thead>
              <tr className="border-b border-line text-left text-ink-muted">
                <th className="px-4 py-2 font-normal">at</th>
                <th className="px-4 py-2 font-normal">actor</th>
                <th className="px-4 py-2 font-normal">action</th>
                <th className="px-4 py-2 font-normal">subject</th>
              </tr>
            </thead>
            <tbody>
              {rows.map((row) => (
                <tr key={row.id} className="border-b border-line/50 align-top">
                  <td className="whitespace-nowrap px-4 py-2 text-ink-muted">
                    {row.occurred_at.toISOString().replace('T', ' ').slice(0, 19)}
                  </td>
                  <td className="px-4 py-2 text-ink-secondary">
                    {row.actor_name ?? 'system'}
                  </td>
                  <td className="px-4 py-2 text-ink">{row.action}</td>
                  <td className="px-4 py-2 text-ink-muted">
                    {row.subject_type}
                    {row.subject_id ? ` ${row.subject_id.slice(0, 8)}` : ''}
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}
    </div>
  );
}
