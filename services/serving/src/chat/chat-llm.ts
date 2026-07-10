import Anthropic from '@anthropic-ai/sdk';
import { Logger } from '@nestjs/common';

export interface ChatTool {
  name: string;
  description: string;
  inputSchema: Record<string, unknown>;
  execute(input: Record<string, unknown>): Promise<unknown>;
}

export interface ChatTurnRequest {
  model: string;
  system: string;
  maxTokens: number;
  messages: { role: 'user' | 'assistant'; content: string }[];
  tools: ChatTool[];
}

export interface ChatTurnResult {
  text: string;
  toolCalls: {
    name: string;
    input: Record<string, unknown>;
    result: unknown;
  }[];
}

export interface ChatLlm {
  readonly enabled: boolean;
  runTurn(request: ChatTurnRequest): Promise<ChatTurnResult>;
}

export const CHAT_LLM = Symbol('CHAT_LLM');

const MAX_TOOL_ROUNDS = 8;

export class AnthropicChatLlm implements ChatLlm {
  readonly enabled = true;

  private readonly logger = new Logger(AnthropicChatLlm.name);
  private readonly client: Anthropic;

  constructor(apiKey: string) {
    this.client = new Anthropic({ apiKey });
  }

  async runTurn(request: ChatTurnRequest): Promise<ChatTurnResult> {
    const toolCalls: ChatTurnResult['toolCalls'] = [];
    const toolsByName = new Map(request.tools.map((t) => [t.name, t]));
    const messages: Anthropic.MessageParam[] = request.messages.map((m) => ({
      role: m.role,
      content: m.content,
    }));
    const tools: Anthropic.Tool[] = request.tools.map((t) => ({
      name: t.name,
      description: t.description,
      input_schema: t.inputSchema as Anthropic.Tool.InputSchema,
    }));

    for (let round = 0; round <= MAX_TOOL_ROUNDS; round++) {
      const response = await this.client.messages.create({
        model: request.model,
        max_tokens: request.maxTokens,
        system: request.system,
        messages,
        tools,
      });

      const toolUses = response.content.filter(
        (block): block is Anthropic.ToolUseBlock => block.type === 'tool_use',
      );
      if (toolUses.length === 0 || round === MAX_TOOL_ROUNDS) {
        const text = response.content
          .filter(
            (block): block is Anthropic.TextBlock => block.type === 'text',
          )
          .map((block) => block.text)
          .join('');
        return { text, toolCalls };
      }

      messages.push({ role: 'assistant', content: response.content });
      const results: Anthropic.ToolResultBlockParam[] = [];
      for (const use of toolUses) {
        const tool = toolsByName.get(use.name);
        let result: unknown;
        let isError = false;
        if (!tool) {
          result = { error: `unknown tool ${use.name}` };
          isError = true;
        } else {
          try {
            result = await tool.execute(use.input as Record<string, unknown>);
          } catch (error) {
            result = { error: String(error) };
            isError = true;
            this.logger.warn(
              JSON.stringify({ event: 'chat_tool_failed', tool: use.name }),
            );
          }
        }
        toolCalls.push({
          name: use.name,
          input: use.input as Record<string, unknown>,
          result,
        });
        results.push({
          type: 'tool_result',
          tool_use_id: use.id,
          content: JSON.stringify(result),
          is_error: isError,
        });
      }
      messages.push({ role: 'user', content: results });
    }
    return { text: '', toolCalls };
  }
}

export class DisabledChatLlm implements ChatLlm {
  readonly enabled = false;

  runTurn(): Promise<ChatTurnResult> {
    return Promise.reject(
      new Error('chat requires ANTHROPIC_API_KEY to be configured'),
    );
  }
}
