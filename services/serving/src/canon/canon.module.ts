import { Module } from '@nestjs/common';

import { AuthModule } from '../auth/auth.module';
import { SERVING_CONFIG } from '../config';
import type { ServingConfig } from '../config';
import { DriftModule } from '../drift/drift.module';
import { CanonController } from './canon.controller';
import { CanonService } from './canon.service';
import { EMBEDDER, FakeEmbedder } from './embedder';

@Module({
  imports: [AuthModule, DriftModule],
  controllers: [CanonController],
  providers: [
    {
      provide: EMBEDDER,
      inject: [SERVING_CONFIG],
      useFactory: (config: ServingConfig) =>
        new FakeEmbedder(config.embeddingModel),
    },
    CanonService,
  ],
  exports: [EMBEDDER, CanonService],
})
export class CanonModule {}
