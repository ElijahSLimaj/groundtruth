export type TrustLabel = 'verified' | 'stale' | 'stream' | 'conflict' | 'none';

export const TRUST_COPY: Record<TrustLabel, string> = {
  verified: 'Verified',
  stale: 'Stale',
  stream: 'Stream signal',
  conflict: 'Conflict',
  none: 'No coverage',
};

export const TRUST_COLOR: Record<TrustLabel, string> = {
  verified: 'text-verified',
  stale: 'text-stale',
  stream: 'text-stream',
  conflict: 'text-conflict',
  none: 'text-nocover',
};

export const TRUST_DOT: Record<TrustLabel, string> = {
  verified: 'bg-verified',
  stale: 'bg-stale',
  stream: 'bg-stream',
  conflict: 'bg-conflict',
  none: 'bg-nocover',
};

export function entryTrust(status: string): TrustLabel {
  if (status === 'active') {
    return 'verified';
  }
  if (status === 'decayed') {
    return 'stale';
  }
  return 'none';
}

export function formatDate(value: Date | string | null): string {
  if (!value) {
    return 'never';
  }
  const date = typeof value === 'string' ? new Date(value) : value;
  return date.toISOString().slice(0, 10);
}
