import { TRUST_COPY, TRUST_DOT, TrustLabel } from '../lib/trust';

export function TrustBadge({ label }: { label: TrustLabel }) {
  return (
    <span className="inline-flex items-center gap-1.5">
      <span
        aria-hidden
        className={`h-1.5 w-1.5 rounded-full ${TRUST_DOT[label]}`}
      />
      <span className="eyebrow text-ink-secondary">{TRUST_COPY[label]}</span>
    </span>
  );
}
