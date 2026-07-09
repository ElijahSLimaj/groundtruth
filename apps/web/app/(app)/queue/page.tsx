import { AttributesDiff, DiffView } from '../../../components/diff-view';
import { EmptyState, PageHeader } from '../../../components/page-header';
import { loadQueue } from '../../../lib/queue';
import { requireViewer } from '../../../lib/session';
import { formatDate } from '../../../lib/trust';
import { approveProposal, rejectProposal } from './actions';
import { QueueKeys } from './queue-keys';

export const dynamic = 'force-dynamic';

const REJECT_REASONS = [
  ['wrong', 'Wrong'],
  ['duplicate', 'Duplicate'],
  ['not_canon_worthy', 'Not canon worthy'],
  ['bad_draft', 'Bad draft'],
  ['other', 'Other'],
] as const;

export default async function QueuePage() {
  const viewer = await requireViewer();
  const { pending, queuedCount } = await loadQueue(viewer);
  const current = pending[0];

  return (
    <div className="max-w-4xl">
      <PageHeader
        title="Approval queue"
        subtitle={
          pending.length === 0
            ? 'The canon is current.'
            : `${pending.length} pending${queuedCount > 0 ? `, ${queuedCount} queued behind the weekly budget` : ''}`
        }
      />

      {!current ? (
        <EmptyState
          title="Nothing waiting on you"
          detail="New drift proposals and cold start drafts will appear here."
        />
      ) : (
        <article className="rounded-card border border-line bg-surface shadow-panel">
          <QueueKeys />
          <div className="flex items-center justify-between border-b border-line px-6 py-3">
            <p className="eyebrow text-ink-secondary">
              {current.kind}
              {current.conflictingField ? ` · ${current.conflictingField}` : ''} ·{' '}
              {current.domain ?? 'no domain'} ·{' '}
              {Math.round(current.confidence * 100)}% confidence
            </p>
            <div className="flex items-center gap-2">
              {current.strategic ? (
                <span className="eyebrow text-conflict">strategic</span>
              ) : null}
              {current.recurring ? (
                <span className="eyebrow text-conflict">
                  recurring after rejection
                </span>
              ) : null}
              <span className="eyebrow text-ink-muted">{current.origin}</span>
            </div>
          </div>

          <div className="grid grid-cols-[1fr_18rem] gap-0">
            <div className="px-6 py-6 flex flex-col gap-5">
              <DiffView
                current={current.currentStatement}
                proposed={current.draftedStatement}
              />
              <AttributesDiff
                current={current.currentAttributes}
                proposed={current.draftedAttributes}
              />

              <div className="mt-2 flex items-center gap-3">
                <form action={approveProposal}>
                  <input type="hidden" name="proposal_id" value={current.id} />
                  <button
                    id="queue-approve"
                    type="submit"
                    className="rounded-control bg-action px-4 py-2 text-sm font-medium text-void hover:opacity-90 focus-visible:outline focus-visible:outline-2 focus-visible:outline-verified"
                  >
                    Approve entry
                  </button>
                </form>
                <details className="group">
                  <summary
                    id="queue-reject-toggle"
                    className="cursor-pointer list-none rounded-control border border-line-strong px-4 py-2 text-sm text-ink-secondary hover:text-ink focus-visible:outline focus-visible:outline-2 focus-visible:outline-action"
                  >
                    Reject with reason
                  </summary>
                  <form
                    action={rejectProposal}
                    className="mt-3 flex flex-col gap-2 rounded-control border border-line bg-raised p-3"
                  >
                    <input type="hidden" name="proposal_id" value={current.id} />
                    <label className="text-xs text-ink-secondary" htmlFor="reason">
                      Reason
                    </label>
                    <select
                      id="reason"
                      name="reason"
                      className="rounded-control border border-line bg-void px-2 py-1.5 text-sm"
                    >
                      {REJECT_REASONS.map(([value, label]) => (
                        <option key={value} value={value}>
                          {label}
                        </option>
                      ))}
                    </select>
                    <label className="text-xs text-ink-secondary" htmlFor="note">
                      Note
                    </label>
                    <input
                      id="note"
                      name="note"
                      type="text"
                      placeholder="What should the drift engine learn?"
                      className="rounded-control border border-line bg-void px-2 py-1.5 text-sm"
                    />
                    <button
                      type="submit"
                      className="mt-1 self-start rounded-control border border-conflict/60 px-3 py-1.5 text-sm text-conflict hover:bg-conflict/10"
                    >
                      Reject
                    </button>
                  </form>
                </details>
                <p className="text-xs text-ink-muted">A approve · X reject</p>
              </div>
            </div>

            <aside className="border-l border-stream/40 bg-void/40 px-4 py-6">
              <p className="eyebrow text-stream mb-3">source excerpts</p>
              {current.evidence.length === 0 ? (
                <p className="text-xs text-ink-muted">
                  No stream evidence attached.
                </p>
              ) : (
                <ul className="flex flex-col gap-3">
                  {current.evidence.map((item, index) => (
                    <li
                      key={index}
                      className="rounded-control border border-line bg-surface p-3"
                    >
                      <p className="font-mono text-xs text-ink-secondary whitespace-pre-wrap break-words">
                        {item.content.slice(0, 400)}
                      </p>
                      <p className="mt-2 eyebrow text-ink-muted">
                        {formatDate(item.occurredAt)}
                      </p>
                    </li>
                  ))}
                </ul>
              )}
            </aside>
          </div>
        </article>
      )}

      {pending.length > 1 ? (
        <p className="mt-4 text-sm text-ink-muted">
          {pending.length - 1} more after this one.
        </p>
      ) : null}
    </div>
  );
}
