import {
  calibrateConfidence,
  decodeTier3,
  resolveTuning,
  Tier2Result,
  Tier3Wire,
} from './schemas';

describe('Tier2Result schema', () => {
  it('accepts a valid classification', () => {
    const parsed = Tier2Result.parse({
      relation: 'contradicts',
      confidence: 0.9,
      conflicting_field: 'amount',
    });
    expect(parsed.relation).toBe('contradicts');
  });

  it('rejects unknown relations', () => {
    expect(() =>
      Tier2Result.parse({
        relation: 'maybe',
        confidence: 0.5,
        conflicting_field: null,
      }),
    ).toThrow();
  });

  it('rejects out of range confidence', () => {
    expect(() =>
      Tier2Result.parse({
        relation: 'confirms',
        confidence: 1.4,
        conflicting_field: null,
      }),
    ).toThrow();
  });
});

describe('decodeTier3', () => {
  const wire = (attributesJson: string) =>
    Tier3Wire.parse({
      drafted_statement: 'Growth is 1799 per month',
      drafted_attributes_json: attributesJson,
      contradiction_description: 'price changed',
      supporting_excerpts: ['1799 from August'],
      confidence: 0.8,
    });

  it('decodes valid attribute JSON', () => {
    const result = decodeTier3(wire('{"amount": 1799}'));
    expect(result?.draftedAttributes).toEqual({ amount: 1799 });
  });

  it('returns null for malformed JSON', () => {
    expect(decodeTier3(wire('{not json'))).toBeNull();
  });

  it('returns null for non object attributes', () => {
    expect(decodeTier3(wire('[1, 2]'))).toBeNull();
    expect(decodeTier3(wire('"text"'))).toBeNull();
  });
});

describe('resolveTuning', () => {
  it('applies defaults for missing params', () => {
    const tuning = resolveTuning(undefined);
    expect(tuning.tier1_threshold).toBe(0.78);
    expect(tuning.owner_weekly_budget).toBe(10);
  });

  it('overrides only what the tenant configures', () => {
    const tuning = resolveTuning({ tier2_gate: 0.85 });
    expect(tuning.tier2_gate).toBe(0.85);
    expect(tuning.cooldown_days).toBe(14);
  });
});

describe('calibrateConfidence', () => {
  it('returns raw confidence on the global identity curve', () => {
    expect(calibrateConfidence(0.83, null)).toBe(0.83);
  });

  it('maps raw confidence through its bin rate', () => {
    const curve = [0.1, 0.2, 0.4, 0.6, 0.9];
    expect(calibrateConfidence(0.05, curve)).toBe(0.1);
    expect(calibrateConfidence(0.55, curve)).toBe(0.4);
    expect(calibrateConfidence(1, curve)).toBe(0.9);
  });

  it('falls back to raw confidence in unsampled bins', () => {
    const curve = [0.1, null, null, null, 0.9];
    expect(calibrateConfidence(0.5, curve)).toBe(0.5);
    expect(calibrateConfidence(0.95, curve)).toBe(0.9);
  });

  it('ignores malformed curves', () => {
    expect(calibrateConfidence(0.7, [0.5])).toBe(0.7);
  });
});
