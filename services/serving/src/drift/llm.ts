import Anthropic from '@anthropic-ai/sdk';
import { zodOutputFormat } from '@anthropic-ai/sdk/helpers/zod';
import { Logger } from '@nestjs/common';
import { z } from 'zod';

export interface LlmJsonRequest {
  model: string;
  system: string;
  user: string;
  maxTokens: number;
  promptVersion: string;
}

export interface LlmClient {
  readonly enabled: boolean;
  completeJson<T>(request: LlmJsonRequest, schema: z.ZodType<T>): Promise<T>;
}

export const LLM_CLIENT = Symbol('LLM_CLIENT');

const MAX_ATTEMPTS = 3;

export class AnthropicLlmClient implements LlmClient {
  readonly enabled = true;

  private readonly logger = new Logger(AnthropicLlmClient.name);
  private readonly client: Anthropic;

  constructor(apiKey: string) {
    this.client = new Anthropic({ apiKey });
  }

  async completeJson<T>(
    request: LlmJsonRequest,
    schema: z.ZodType<T>,
  ): Promise<T> {
    let lastError = '';
    for (let attempt = 1; attempt <= MAX_ATTEMPTS; attempt++) {
      const retryNote = lastError
        ? `\n\nYour previous response failed validation: ${lastError}. Return output matching the required schema exactly.`
        : '';
      const started = Date.now();
      const response = await this.client.messages.parse({
        model: request.model,
        max_tokens: request.maxTokens,
        system: request.system,
        messages: [{ role: 'user', content: request.user + retryNote }],
        output_config: { format: zodOutputFormat(schema) },
      });
      this.logger.log(
        JSON.stringify({
          event: 'llm_call',
          model: request.model,
          prompt_version: request.promptVersion,
          attempt,
          latency_ms: Date.now() - started,
          input_tokens: response.usage.input_tokens,
          output_tokens: response.usage.output_tokens,
          stop_reason: response.stop_reason,
        }),
      );

      if (response.stop_reason === 'refusal') {
        throw new Error(`llm refused ${request.promptVersion} request`);
      }
      if (response.parsed_output != null) {
        return response.parsed_output;
      }
      lastError = 'output did not match the required schema';
    }
    throw new Error(
      `llm output failed validation after ${MAX_ATTEMPTS} attempts for ${request.promptVersion}`,
    );
  }
}

export class DisabledLlmClient implements LlmClient {
  readonly enabled = false;

  completeJson(): Promise<never> {
    return Promise.reject(
      new Error('drift engine is disabled, ANTHROPIC_API_KEY is not set'),
    );
  }
}
