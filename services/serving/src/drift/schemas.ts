import { z } from 'zod';

export const Tier2Result = z.object({
  relation: z.enum(['contradicts', 'confirms', 'extends', 'unrelated']),
  confidence: z.number().min(0).max(1),
  conflicting_field: z.string().nullable(),
});
export type Tier2Result = z.infer<typeof Tier2Result>;

export const SourcedExcerpt = z.object({
  source_id: z.string().min(1),
  text: z.string().min(1),
});
export type SourcedExcerpt = z.infer<typeof SourcedExcerpt>;

export interface AttributedExcerpt {
  chunkId: string;
  text: string;
}

export function attributeExcerpts(
  excerpts: SourcedExcerpt[],
  chunkIdBySourceId: Map<string, string>,
): AttributedExcerpt[] {
  return excerpts.flatMap((excerpt) => {
    const chunkId = chunkIdBySourceId.get(excerpt.source_id);
    return chunkId ? [{ chunkId, text: excerpt.text }] : [];
  });
}

export const Tier3Wire = z.object({
  drafted_statement: z.string().min(1),
  drafted_attributes_json: z.string(),
  contradiction_description: z.string().min(1),
  supporting_excerpts: z.array(SourcedExcerpt).max(5),
  confidence: z.number().min(0).max(1),
});
export type Tier3Wire = z.infer<typeof Tier3Wire>;

export interface Tier3Result {
  draftedStatement: string;
  draftedAttributes: Record<string, unknown>;
  contradictionDescription: string;
  supportingExcerpts: SourcedExcerpt[];
  confidence: number;
}

export function decodeTier3(wire: Tier3Wire): Tier3Result | null {
  let attributes: unknown;
  try {
    attributes = JSON.parse(wire.drafted_attributes_json);
  } catch {
    return null;
  }
  if (
    typeof attributes !== 'object' ||
    attributes === null ||
    Array.isArray(attributes)
  ) {
    return null;
  }
  return {
    draftedStatement: wire.drafted_statement,
    draftedAttributes: attributes as Record<string, unknown>,
    contradictionDescription: wire.contradiction_description,
    supportingExcerpts: wire.supporting_excerpts,
    confidence: wire.confidence,
  };
}

export const GapTier2Result = z.object({
  canon_worthy: z.boolean(),
  domain: z.string().min(1),
  confidence: z.number().min(0).max(1),
});
export type GapTier2Result = z.infer<typeof GapTier2Result>;

export const GapTier3Wire = z.object({
  drafted_statement: z.string().min(1),
  drafted_attributes_json: z.string(),
  gap_description: z.string().min(1),
  supporting_excerpts: z.array(SourcedExcerpt).max(5),
  confidence: z.number().min(0).max(1),
});
export type GapTier3Wire = z.infer<typeof GapTier3Wire>;

export interface GapTier3Result {
  draftedStatement: string;
  draftedAttributes: Record<string, unknown>;
  gapDescription: string;
  supportingExcerpts: SourcedExcerpt[];
  confidence: number;
}

export function decodeGapTier3(wire: GapTier3Wire): GapTier3Result | null {
  let attributes: unknown;
  try {
    attributes = JSON.parse(wire.drafted_attributes_json);
  } catch {
    return null;
  }
  if (
    typeof attributes !== 'object' ||
    attributes === null ||
    Array.isArray(attributes)
  ) {
    return null;
  }
  return {
    draftedStatement: wire.drafted_statement,
    draftedAttributes: attributes as Record<string, unknown>,
    gapDescription: wire.gap_description,
    supportingExcerpts: wire.supporting_excerpts,
    confidence: wire.confidence,
  };
}

export interface TuningParams {
  tier1_threshold: number;
  tier1_source_discounts: Record<string, number>;
  tier1_domain_penalties: Record<string, number>;
  tier2_gate: number;
  owner_weekly_budget: number;
  cooldown_days: number;
  scan_limit: number;
  gap_similarity: number;
  gap_cluster_min_size: number;
  gap_min_authors: number;
  gap_buffer_days: number;
  tier3_calibration: (number | null)[] | null;
}

export const DEFAULT_TUNING: TuningParams = {
  tier1_threshold: 0.78,
  tier1_source_discounts: { crm: 0.08, transcript: 0.08 },
  tier1_domain_penalties: {},
  tier2_gate: 0.7,
  owner_weekly_budget: 10,
  cooldown_days: 14,
  scan_limit: 200,
  gap_similarity: 0.83,
  gap_cluster_min_size: 5,
  gap_min_authors: 2,
  gap_buffer_days: 30,
  tier3_calibration: null,
};

export const CALIBRATION_BINS = 5;

export function calibrateConfidence(
  raw: number,
  curve: (number | null)[] | null,
): number {
  if (!curve || curve.length !== CALIBRATION_BINS) {
    return raw;
  }
  const bin = Math.min(
    CALIBRATION_BINS - 1,
    Math.floor(raw * CALIBRATION_BINS),
  );
  const rate = curve[bin];
  return rate === null ? raw : rate;
}

export function resolveTuning(raw: unknown): TuningParams {
  const params = (raw ?? {}) as Partial<TuningParams>;
  return {
    tier1_threshold: params.tier1_threshold ?? DEFAULT_TUNING.tier1_threshold,
    tier1_source_discounts:
      params.tier1_source_discounts ?? DEFAULT_TUNING.tier1_source_discounts,
    tier1_domain_penalties:
      params.tier1_domain_penalties ?? DEFAULT_TUNING.tier1_domain_penalties,
    tier2_gate: params.tier2_gate ?? DEFAULT_TUNING.tier2_gate,
    owner_weekly_budget:
      params.owner_weekly_budget ?? DEFAULT_TUNING.owner_weekly_budget,
    cooldown_days: params.cooldown_days ?? DEFAULT_TUNING.cooldown_days,
    scan_limit: params.scan_limit ?? DEFAULT_TUNING.scan_limit,
    gap_similarity: params.gap_similarity ?? DEFAULT_TUNING.gap_similarity,
    gap_cluster_min_size:
      params.gap_cluster_min_size ?? DEFAULT_TUNING.gap_cluster_min_size,
    gap_min_authors: params.gap_min_authors ?? DEFAULT_TUNING.gap_min_authors,
    gap_buffer_days: params.gap_buffer_days ?? DEFAULT_TUNING.gap_buffer_days,
    tier3_calibration:
      params.tier3_calibration ?? DEFAULT_TUNING.tier3_calibration,
  };
}
