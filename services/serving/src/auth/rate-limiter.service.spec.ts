import { RateLimiterService } from './rate-limiter.service';

describe('RateLimiterService', () => {
  let limiter: RateLimiterService;
  let now: number;

  beforeEach(() => {
    limiter = new RateLimiterService();
    now = 1_000_000;
    limiter.nowFn = () => now;
  });

  it('allows up to the burst capacity', () => {
    expect(limiter.take('k1', 'minimal').allowed).toBe(true);
    expect(limiter.take('k1', 'minimal').allowed).toBe(true);
  });

  it('rejects past the burst with a retry-after', () => {
    limiter.take('k1', 'minimal');
    limiter.take('k1', 'minimal');
    const verdict = limiter.take('k1', 'minimal');
    expect(verdict.allowed).toBe(false);
    expect(verdict.retryAfterSeconds).toBeGreaterThan(0);
  });

  it('refills over time', () => {
    limiter.take('k1', 'minimal');
    limiter.take('k1', 'minimal');
    expect(limiter.take('k1', 'minimal').allowed).toBe(false);
    now += 61_000;
    expect(limiter.take('k1', 'minimal').allowed).toBe(true);
  });

  it('tracks keys independently', () => {
    limiter.take('k1', 'minimal');
    limiter.take('k1', 'minimal');
    expect(limiter.take('k1', 'minimal').allowed).toBe(false);
    expect(limiter.take('k2', 'minimal').allowed).toBe(true);
  });

  it('falls back to the standard tier for unknown tiers', () => {
    for (let i = 0; i < 30; i++) {
      expect(limiter.take('k1', 'nonsense').allowed).toBe(true);
    }
    expect(limiter.take('k1', 'nonsense').allowed).toBe(false);
  });
});
