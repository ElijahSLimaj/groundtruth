import { Module } from '@nestjs/common';

import { SERVING_CONFIG } from '../config';
import type { ServingConfig } from '../config';
import { DriftService } from './drift.service';
import { AnthropicLlmClient, DisabledLlmClient, LLM_CLIENT } from './llm';

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
  ],
  exports: [DriftService, LLM_CLIENT],
})
export class DriftModule {}
