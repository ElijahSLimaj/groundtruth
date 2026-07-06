export const PERSON_ROLES = ['admin', 'owner', 'member', 'agent'] as const;
export type PersonRole = (typeof PERSON_ROLES)[number];

export const CANON_TIERS = ['bedrock', 'operational'] as const;
export type CanonTier = (typeof CANON_TIERS)[number];

export const ENTRY_STATUSES = ['active', 'decayed', 'archived'] as const;
export type EntryStatus = (typeof ENTRY_STATUSES)[number];

export const CANON_RELATIONS = ['supersedes', 'conflicts_with', 'depends_on'] as const;
export type CanonRelation = (typeof CANON_RELATIONS)[number];

export const DRIFT_KINDS = ['contradiction', 'gap', 'decay'] as const;
export type DriftKind = (typeof DRIFT_KINDS)[number];

export const APPROVAL_DECISIONS = ['approved', 'rejected'] as const;
export type ApprovalDecision = (typeof APPROVAL_DECISIONS)[number];

export const ACL_SCOPES = ['principals', 'group', 'tenant'] as const;
export type AclScope = (typeof ACL_SCOPES)[number];

export interface SourceScope {
  type: string;
  id: string;
  visibility?: string;
}

export interface Acl {
  scope: AclScope;
  principals?: string[];
  source_scope?: SourceScope;
}

export const TRUST_LABELS = ['verified', 'stale', 'stream', 'conflict', 'none'] as const;
export type TrustLabel = (typeof TRUST_LABELS)[number];

export const TRUST_LABEL_COPY: Record<TrustLabel, string> = {
  verified: 'Verified',
  stale: 'Stale',
  stream: 'Stream signal',
  conflict: 'Conflict',
  none: 'No coverage',
};
