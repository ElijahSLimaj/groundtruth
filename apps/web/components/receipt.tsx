import { formatDate } from '../lib/trust';

export interface ReceiptData {
  approver: string | null;
  verifiedAt: Date | string | null;
  sourceCount: number;
}

export function Receipt({ receipt }: { receipt: ReceiptData }) {
  return (
    <p className="font-mono text-xs text-ink-muted">
      <span aria-hidden className="text-verified">
        ⬢{' '}
      </span>
      {receipt.approver ?? 'unapproved'} · verified {formatDate(receipt.verifiedAt)} ·{' '}
      {receipt.sourceCount} source{receipt.sourceCount === 1 ? '' : 's'}
    </p>
  );
}
