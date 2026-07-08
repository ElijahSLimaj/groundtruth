import { Module } from '@nestjs/common';

import { SERVING_CONFIG } from '../config';
import type { ServingConfig } from '../config';
import { DriftService } from './drift.service';
import { GapService } from './gap.service';
import { AnthropicLlmClient, DisabledLlmClient, LLM_CLIENT } from './llm';
import { MergeService } from './merge.service';
import { TuningService } from './tuning.service';

@Module({
  providers: [
    {
      provide: LLM_CLIENT,
      inject: [SERVING_CONFIG],
      useFactory: (config: ServingConfig) =>
        config.anthropicApiKey
          ? new AnthropicLlmClient(config.anthropicApiKey)
          : new DisabledLlmClient(),
    },
    DriftService,
    GapService,
    MergeService,
    TuningService,
  ],
  exports: [DriftService, GapService, MergeService, TuningService, LLM_CLIENT],
})
export class DriftModule {}
