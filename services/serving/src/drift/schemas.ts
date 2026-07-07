import { z } from 'zod';

export const Tier2Result = z.object({
  relation: z.enum(['contradicts', 'confirms', 'extends', 'unrelated']),
  confidence: z.number().min(0).max(1),
  conflicting_field: z.string().nullable(),
});
export type Tier2Result = z.infer<typeof Tier2Result>;

export const Tier3Wire = z.object({
  drafted_statement: z.string().min(1),
  drafted_attributes_json: z.string(),
  contradiction_description: z.string().min(1),
  supporting_excerpts: z.array(z.string()).max(5),
  confidence: z.number().min(0).max(1),
});
export type Tier3Wire = z.infer<typeof Tier3Wire>;

export interface Tier3Result {
  draftedStatement: string;
  draftedAttributes: Record<string, unknown>;
  contradictionDescription: string;
  supportingExcerpts: string[];
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

export interface TuningParams {
  tier1_threshold: number;
  tier1_source_discounts: Record<string, number>;
  tier2_gate: number;
  owner_weekly_budget: number;
  cooldown_days: number;
  scan_limit: number;
}

export const DEFAULT_TUNING: TuningParams = {
  tier1_threshold: 0.78,
  tier1_source_discounts: { crm: 0.08, transcript: 0.08 },
  tier2_gate: 0.7,
  owner_weekly_budget: 10,
  cooldown_days: 14,
  scan_limit: 200,
};

export function resolveTuning(raw: unknown): TuningParams {
  const params = (raw ?? {}) as Partial<TuningParams>;
  return {
    tier1_threshold: params.tier1_threshold ?? DEFAULT_TUNING.tier1_threshold,
    tier1_source_discounts:
      params.tier1_source_discounts ?? DEFAULT_TUNING.tier1_source_discounts,
    tier2_gate: params.tier2_gate ?? DEFAULT_TUNING.tier2_gate,
    owner_weekly_budget:
      params.owner_weekly_budget ?? DEFAULT_TUNING.owner_weekly_budget,
    cooldown_days: params.cooldown_days ?? DEFAULT_TUNING.cooldown_days,
    scan_limit: params.scan_limit ?? DEFAULT_TUNING.scan_limit,
  };
}
