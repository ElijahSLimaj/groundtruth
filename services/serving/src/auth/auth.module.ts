import { Module } from '@nestjs/common';

import { ApiKeyGuard } from './api-key.guard';
import { RateLimiterService } from './rate-limiter.service';

@Module({
  providers: [ApiKeyGuard, RateLimiterService],
  exports: [ApiKeyGuard, RateLimiterService],
})
export class AuthModule {}
