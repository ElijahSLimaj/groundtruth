import { PageHeader } from '../../../components/page-header';
import { brainFetch } from '../../../lib/brain-api';
import { requireViewer } from '../../../lib/session';
import { issueKey, openBillingPortal, revokeKey } from './actions';

export const dynamic = 'force-dynamic';

interface Overview {
  plan: string | null;
  subscription_status: string | null;
  included_query_volume: number | null;
  usage_this_month: number;
  keys: { id: string; name: string; rate_tier: string; created_at: string }[];
}

export default async function SettingsPage({
  searchParams,
}: {
  searchParams: Promise<{ new_key?: string; error?: string }>;
}) {
  const viewer = await requireViewer();
  const { new_key, error } = await searchParams;
  const canManage = viewer.role === 'admin' || viewer.role === 'owner';

  let overview: Overview | null = null;
  try {
    overview = await brainFetch<Overview>(viewer, '/account');
  } catch {
    overview = null;
  }

  return (
    <div className="max-w-3xl">
      <PageHeader title="Settings" subtitle="Plan, billing, and agent keys." />

      {new_key ? (
        <div className="mb-6 rounded-card border border-verified/40 bg-verified/10 px-4 py-3">
          <p className="text-sm text-ink">
            New agent key — copy it now, it is not shown again.
          </p>
          <code className="mt-2 block break-all font-mono text-xs text-verified">
            {new_key}
          </code>
        </div>
      ) : null}
      {error === 'billing' ? (
        <p className="mb-4 rounded-card border border-conflict/40 bg-conflict/10 px-4 py-2 text-sm text-conflict">
          Billing portal unavailable. Stripe may not be configured, or this
          workspace has no subscription yet.
        </p>
      ) : null}

      <section className="rounded-card border border-line bg-surface px-5 py-4">
        <p className="eyebrow text-ink-muted">Plan</p>
        <div className="mt-2 flex items-center justify-between">
          <div>
            <p className="text-md text-ink capitalize">
              {overview?.plan ?? 'not set'}
              {overview?.subscription_status
                ? ` · ${overview.subscription_status}`
                : ''}
            </p>
            <p className="mt-1 font-mono text-xs text-ink-muted">
              {overview
                ? `${overview.usage_this_month.toLocaleString()} agent queries this month${
                    overview.included_query_volume
                      ? ` of ${overview.included_query_volume.toLocaleString()} included`
                      : ''
                  }`
                : 'usage unavailable'}
            </p>
          </div>
          {canManage ? (
            <form action={openBillingPortal}>
              <button
                type="submit"
                className="rounded-control border border-line-strong px-3 py-1.5 text-sm text-ink-secondary hover:text-ink"
              >
                Manage billing
              </button>
            </form>
          ) : null}
        </div>
      </section>

      <section className="mt-6">
        <p className="eyebrow text-ink-muted mb-3">Agent keys</p>
        {canManage ? (
          <form
            action={issueKey}
            className="mb-4 flex items-end gap-2 rounded-card border border-line bg-surface px-4 py-3"
          >
            <div className="flex-1">
              <label className="text-xs text-ink-secondary">Name</label>
              <input
                name="name"
                required
                placeholder="Sales agent"
                className="mt-1 w-full rounded-control border border-line bg-void px-2 py-1.5 text-sm"
              />
            </div>
            <select
              name="rate_tier"
              defaultValue="standard"
              className="rounded-control border border-line bg-void px-2 py-1.5 text-sm"
            >
              <option value="standard">standard</option>
              <option value="high">high</option>
              <option value="minimal">minimal</option>
            </select>
            <button
              type="submit"
              className="rounded-control bg-action px-3 py-1.5 text-sm font-medium text-void hover:opacity-90"
            >
              Issue key
            </button>
          </form>
        ) : null}

        {overview && overview.keys.length > 0 ? (
          <div className="rounded-card border border-line">
            {overview.keys.map((key) => (
              <div
                key={key.id}
                className="flex items-center justify-between border-b border-line/50 px-4 py-3 last:border-0"
              >
                <div>
                  <p className="text-sm text-ink">{key.name}</p>
                  <p className="font-mono text-xs text-ink-muted">
                    {key.rate_tier} · issued{' '}
                    {new Date(key.created_at).toISOString().slice(0, 10)}
                  </p>
                </div>
                {canManage ? (
                  <form action={revokeKey}>
                    <input type="hidden" name="id" value={key.id} />
                    <button
                      type="submit"
                      className="text-xs text-ink-muted underline hover:text-conflict"
                    >
                      Revoke
                    </button>
                  </form>
                ) : null}
              </div>
            ))}
          </div>
        ) : (
          <p className="text-sm text-ink-muted">No agent keys yet.</p>
        )}
      </section>
    </div>
  );
}
