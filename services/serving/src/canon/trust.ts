export type TrustLevel =
  'canon_verified' | 'canon_stale' | 'stream_only' | 'no_coverage';

export function deriveTrust(entryStatuses: string[]): TrustLevel {
  if (entryStatuses.length === 0) {
    return 'no_coverage';
  }
  return entryStatuses.some((status) => status === 'decayed')
    ? 'canon_stale'
    : 'canon_verified';
}
