import { Module } from '@nestjs/common';

import { AuthModule } from '../auth/auth.module';
import { CanonModule } from '../canon/canon.module';
import { SERVING_CONFIG } from '../config';
import type { ServingConfig } from '../config';
import { InternalGuard } from '../auth/internal.guard';
import { ChatController } from './chat.controller';
import { AnthropicChatLlm, CHAT_LLM, DisabledChatLlm } from './chat-llm';
import { ChatService } from './chat.service';

@Module({
  imports: [AuthModule, CanonModule],
  controllers: [ChatController],
  providers: [
    {
      provide: CHAT_LLM,
      inject: [SERVING_CONFIG],
      useFactory: (config: ServingConfig) =>
        config.anthropicApiKey
          ? new AnthropicChatLlm(config.anthropicApiKey)
          : new DisabledChatLlm(),
    },
    ChatService,
    InternalGuard,
  ],
})
export class ChatModule {}
