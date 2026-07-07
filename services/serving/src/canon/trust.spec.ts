import { deriveTrust } from './trust';

describe('deriveTrust', () => {
  it('returns no_coverage when nothing is cited', () => {
    expect(deriveTrust([])).toBe('no_coverage');
  });

  it('returns canon_verified when every cited entry is active', () => {
    expect(deriveTrust(['active', 'active'])).toBe('canon_verified');
  });

  it('returns canon_stale when any cited entry has decayed', () => {
    expect(deriveTrust(['active', 'decayed'])).toBe('canon_stale');
  });

  it('returns canon_stale when the only citation has decayed', () => {
    expect(deriveTrust(['decayed'])).toBe('canon_stale');
  });
});
