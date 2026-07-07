import { Injectable } from '@nestjs/common';

interface TierConfig {
  capacity: number;
  refillPerSecond: number;
}

interface Bucket {
  tokens: number;
  updatedAt: number;
}

const TIERS: Record<string, TierConfig> = {
  standard: { capacity: 30, refillPerSecond: 1 },
  high: { capacity: 100, refillPerSecond: 10 },
  minimal: { capacity: 2, refillPerSecond: 1 / 60 },
};

export interface RateVerdict {
  allowed: boolean;
  retryAfterSeconds: number;
}

@Injectable()
export class RateLimiterService {
  nowFn: () => number = Date.now;

  private readonly buckets = new Map<string, Bucket>();

  take(keyId: string, tier: string): RateVerdict {
    const config = TIERS[tier] ?? TIERS['standard'];
    const now = this.nowFn();
    const bucket = this.buckets.get(keyId) ?? {
      tokens: config.capacity,
      updatedAt: now,
    };
    const elapsedSeconds = (now - bucket.updatedAt) / 1000;
    bucket.tokens = Math.min(
      config.capacity,
      bucket.tokens + elapsedSeconds * config.refillPerSecond,
    );
    bucket.updatedAt = now;

    if (bucket.tokens >= 1) {
      bucket.tokens -= 1;
      this.buckets.set(keyId, bucket);
      return { allowed: true, retryAfterSeconds: 0 };
    }
    this.buckets.set(keyId, bucket);
    return {
      allowed: false,
      retryAfterSeconds: Math.ceil(
        (1 - bucket.tokens) / config.refillPerSecond,
      ),
    };
  }
}
