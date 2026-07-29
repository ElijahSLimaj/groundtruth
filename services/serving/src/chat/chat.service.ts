import {
  Inject,
  Injectable,
  NotFoundException,
  ServiceUnavailableException,
} from '@nestjs/common';
import { PoolClient } from 'pg';

import { SERVING_CONFIG } from '../config';
import type { ServingConfig } from '../config';
import type { Principal } from '../auth/principal';
import { CanonService } from '../canon/canon.service';
import { DatabaseService } from '../database/database.service';
import { CHAT_LLM } from './chat-llm';
import type { ChatLlm, ChatTool } from './chat-llm';

const CHAT_SYSTEM = `You are Company Brain, the governed knowledge interface for this company.

Rules:
- Answer company questions only from what query_brain and get_entry return. Never state a company fact from your own knowledge.
- Always report the trust label the brain returned. When the brain returns no_coverage, say the canon does not cover it. Never fill the gap with a guess.
- When the user states a fact that contradicts the canon, offer to file it with propose_update and do so when they confirm.
- When asked for a document or presentation, first retrieve the relevant canon with query_brain, then build it with create_document or create_deck, citing the entry ids each section relies on. Never include an uncited company fact in an artifact.
- Plain verbs, sentence case, no filler.`;

const BILLABLE_CHAT_TOOLS = new Set([
  'propose_update',
  'create_document',
  'create_deck',
]);

export interface ConversationSummary {
  id: string;
  title: string;
  updated_at: string;
}

export interface ChatMessageRow {
  id: string;
  role: string;
  content: string;
  citations: unknown[];
  created_at: string;
}

export interface ChatArtifactRow {
  id: string;
  kind: string;
  title: string;
  content: unknown;
  created_at: string;
}

@Injectable()
export class ChatService {
  constructor(
    private readonly db: DatabaseService,
    private readonly canon: CanonService,
    @Inject(CHAT_LLM) private readonly llm: ChatLlm,
    @Inject(SERVING_CONFIG) private readonly config: ServingConfig,
  ) {}

  async listConversations(
    principal: Principal,
  ): Promise<ConversationSummary[]> {
    return this.db.withTenant(principal.tenantId, async (client) => {
      const rows = await client.query<ConversationSummary>(
        `select id, title, updated_at from chat_conversations
         where person_id = $1 order by updated_at desc limit 50`,
        [principal.personId],
      );
      return rows.rows;
    });
  }

  async getConversation(
    principal: Principal,
    conversationId: string,
  ): Promise<{
    id: string;
    title: string;
    messages: ChatMessageRow[];
    artifacts: ChatArtifactRow[];
  }> {
    return this.db.withTenant(principal.tenantId, async (client) => {
      const conversation = await this.loadOwned(
        client,
        principal,
        conversationId,
      );
      const messages = await client.query<ChatMessageRow>(
        `select id, role, content, citations, created_at from chat_messages
         where conversation_id = $1 order by created_at`,
        [conversationId],
      );
      const artifacts = await client.query<ChatArtifactRow>(
        `select id, kind, title, content, created_at from chat_artifacts
         where conversation_id = $1 order by created_at`,
        [conversationId],
      );
      return {
        id: conversation.id,
        title: conversation.title,
        messages: messages.rows,
        artifacts: artifacts.rows,
      };
    });
  }

  async sendMessage(
    principal: Principal,
    conversationId: string | null,
    content: string,
  ): Promise<{
    conversation_id: string;
    message: ChatMessageRow;
    artifacts: ChatArtifactRow[];
  }> {
    if (!this.llm.enabled) {
      throw new ServiceUnavailableException(
        'chat is disabled until ANTHROPIC_API_KEY is configured',
      );
    }

    const { id, history } = await this.db.withTenant(
      principal.tenantId,
      async (client) => {
        let id = conversationId;
        if (id) {
          await this.loadOwned(client, principal, id);
        } else {
          const created = await client.query<{ id: string }>(
            `insert into chat_conversations (tenant_id, person_id, title)
             values ($1, $2, $3) returning id`,
            [principal.tenantId, principal.personId, content.slice(0, 80)],
          );
          id = created.rows[0].id;
        }
        const prior = await client.query<{ role: string; content: string }>(
          `select role, content from chat_messages
           where conversation_id = $1 order by created_at limit 40`,
          [id],
        );
        await client.query(
          `insert into chat_messages (tenant_id, conversation_id, role, content)
           values ($1, $2, 'user', $3)`,
          [principal.tenantId, id, content],
        );
        return { id, history: prior.rows };
      },
    );

    const citations: unknown[] = [];
    const artifactIds: string[] = [];
    const turn = await this.llm.runTurn({
      model: this.config.chatModel,
      system: CHAT_SYSTEM,
      maxTokens: 4096,
      messages: [
        ...history.map((m) => ({
          role: m.role as 'user' | 'assistant',
          content: m.content,
        })),
        { role: 'user', content },
      ],
      tools: this.buildTools(principal, id, citations, artifactIds),
    });

    const billableTools = [...new Set(turn.toolCalls.map((c) => c.name))]
      .filter((name) => BILLABLE_CHAT_TOOLS.has(name))
      .sort();

    return this.db.withTenant(principal.tenantId, async (client) => {
      const message = await client.query<ChatMessageRow>(
        `insert into chat_messages (tenant_id, conversation_id, role, content, citations)
         values ($1, $2, 'assistant', $3, $4)
         returning id, role, content, citations, created_at`,
        [principal.tenantId, id, turn.text, JSON.stringify(citations)],
      );
      await client.query(
        `insert into metering_events
           (tenant_id, api_key_id, person_id, tool, category, model,
            input_tokens, output_tokens, billable)
         values ($1, null, $2, $3, 'agent_run', $4, $5, $6, $7)`,
        [
          principal.tenantId,
          principal.personId,
          billableTools.length > 0 ? billableTools.join('+') : 'chat.read',
          this.config.chatModel,
          turn.usage.inputTokens,
          turn.usage.outputTokens,
          billableTools.length > 0,
        ],
      );
      await client.query(
        `update chat_artifacts set message_id = $2
         where id = any($3) and conversation_id = $1`,
        [id, message.rows[0].id, artifactIds],
      );
      await client.query(
        `update chat_conversations set updated_at = now() where id = $1`,
        [id],
      );
      const artifacts = await client.query<ChatArtifactRow>(
        `select id, kind, title, content, created_at from chat_artifacts
         where id = any($1) order by created_at`,
        [artifactIds],
      );
      return {
        conversation_id: id,
        message: message.rows[0],
        artifacts: artifacts.rows,
      };
    });
  }

  private buildTools(
    principal: Principal,
    conversationId: string,
    citations: unknown[],
    artifactIds: string[],
  ): ChatTool[] {
    const saveArtifact = async (
      kind: 'document' | 'deck',
      title: string,
      content: unknown,
    ) => {
      return this.db.withTenant(principal.tenantId, async (client) => {
        const inserted = await client.query<{ id: string }>(
          `insert into chat_artifacts (tenant_id, conversation_id, kind, title, content)
           values ($1, $2, $3, $4, $5) returning id`,
          [
            principal.tenantId,
            conversationId,
            kind,
            title,
            JSON.stringify(content),
          ],
        );
        artifactIds.push(inserted.rows[0].id);
        return { artifact_id: inserted.rows[0].id, kind };
      });
    };

    return [
      {
        name: 'query_brain',
        description:
          'Ask the governed company knowledge base a question. Returns an answer with a trust label, citations, conflicts, and freshness.',
        inputSchema: {
          type: 'object',
          properties: {
            question: { type: 'string' },
            domains: { type: 'array', items: { type: 'string' } },
          },
          required: ['question'],
        },
        execute: async (input) => {
          const result = await this.canon.query(principal, {
            question: String(input.question),
            domains: input.domains as string[] | undefined,
          });
          const cited = (result as { citations?: unknown[] }).citations ?? [];
          citations.push(...cited);
          return result;
        },
      },
      {
        name: 'get_entry',
        description:
          'Fetch one canon entry with attributes, version history, and provenance.',
        inputSchema: {
          type: 'object',
          properties: { entry_id: { type: 'string' } },
          required: ['entry_id'],
        },
        execute: (input) =>
          this.canon.getEntry(principal, String(input.entry_id)),
      },
      {
        name: 'list_conflicts',
        description:
          'List open contradictions between the stream and the canon.',
        inputSchema: {
          type: 'object',
          properties: { domain: { type: 'string' } },
        },
        execute: (input) =>
          this.canon.listConflicts(
            principal,
            (input.domain as string | undefined) ?? undefined,
          ),
      },
      {
        name: 'propose_update',
        description:
          'File a correction into the owner approval pipeline when the user reports the canon is wrong.',
        inputSchema: {
          type: 'object',
          properties: {
            entry_id: { type: 'string' },
            domain: { type: 'string' },
            statement: { type: 'string' },
          },
          required: ['statement'],
        },
        execute: (input) =>
          this.canon.proposeUpdate(principal, {
            entry_id: (input.entry_id as string | undefined) ?? undefined,
            domain: (input.domain as string | undefined) ?? undefined,
            statement: String(input.statement),
          }),
      },
      {
        name: 'create_document',
        description:
          'Create a grounded document artifact. Every section must cite the canon entry ids it relies on.',
        inputSchema: {
          type: 'object',
          properties: {
            title: { type: 'string' },
            sections: {
              type: 'array',
              items: {
                type: 'object',
                properties: {
                  heading: { type: 'string' },
                  body: { type: 'string' },
                  entry_ids: { type: 'array', items: { type: 'string' } },
                },
                required: ['heading', 'body', 'entry_ids'],
              },
            },
          },
          required: ['title', 'sections'],
        },
        execute: (input) =>
          saveArtifact('document', String(input.title), {
            sections: input.sections,
          }),
      },
      {
        name: 'create_deck',
        description:
          'Create a grounded slide deck artifact. Every slide must cite the canon entry ids it relies on.',
        inputSchema: {
          type: 'object',
          properties: {
            title: { type: 'string' },
            slides: {
              type: 'array',
              items: {
                type: 'object',
                properties: {
                  title: { type: 'string' },
                  bullets: { type: 'array', items: { type: 'string' } },
                  entry_ids: { type: 'array', items: { type: 'string' } },
                },
                required: ['title', 'bullets', 'entry_ids'],
              },
            },
          },
          required: ['title', 'slides'],
        },
        execute: (input) =>
          saveArtifact('deck', String(input.title), { slides: input.slides }),
      },
    ];
  }

  private async loadOwned(
    client: PoolClient,
    principal: Principal,
    conversationId: string,
  ): Promise<{ id: string; title: string }> {
    const rows = await client.query<{ id: string; title: string }>(
      `select id, title from chat_conversations
       where id = $1 and person_id = $2`,
      [conversationId, principal.personId],
    );
    if (!rows.rows[0]) {
      throw new NotFoundException('conversation not found');
    }
    return rows.rows[0];
  }
}
