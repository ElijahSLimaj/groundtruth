import { randomUUID } from 'node:crypto';

import { INestApplication } from '@nestjs/common';
import { Test } from '@nestjs/testing';
import { Pool } from 'pg';
import request from 'supertest';

import { AppModule } from '../src/app.module';
import { CHAT_LLM } from '../src/chat/chat-llm';
import type {
  ChatLlm,
  ChatTurnRequest,
  ChatTurnResult,
} from '../src/chat/chat-llm';
import { configureApp } from '../src/configure-app';

const databaseUrl = process.env.TEST_DATABASE_URL;
const suite = databaseUrl ? describe : describe.skip;

interface ScriptedTurn {
  invoke: { name: string; input: Record<string, unknown> }[];
  text: string;
}

class FakeChatLlm implements ChatLlm {
  readonly enabled = true;
  queue: ScriptedTurn[] = [];
  seen: unknown[] = [];

  async runTurn(turnRequest: ChatTurnRequest): Promise<ChatTurnResult> {
    const scripted = this.queue.shift();
    if (!scripted) {
      throw new Error('no scripted chat turn');
    }
    const toolCalls: ChatTurnResult['toolCalls'] = [];
    for (const step of scripted.invoke) {
      const tool = turnRequest.tools.find((t) => t.name === step.name);
      if (!tool) {
        throw new Error(`scripted tool ${step.name} not offered`);
      }
      const result = await tool.execute(step.input);
      this.seen.push(result);
      toolCalls.push({ name: step.name, input: step.input, result });
    }
    return { text: scripted.text, toolCalls };
  }
}

suite('chat engine (e2e)', () => {
  let app: INestApplication;
  let admin: Pool;
  let fake: FakeChatLlm;
  let http: Parameters<typeof request>[0];

  const tenantId = randomUUID();
  const ownerId = randomUUID();
  const entryId = randomUUID();
  const versionId = randomUUID();
  const secret = `internal-${randomUUID()}`;

  const headers = {
    'x-internal-secret': secret,
    'x-tenant-id': tenantId,
    'x-person-id': ownerId,
  };

  beforeAll(async () => {
    admin = new Pool({ connectionString: databaseUrl });
    await admin.query(
      `insert into tenants (id, name, tier) values ($1, 'Chat E2E', 'growth')`,
      [tenantId],
    );
    await admin.query(
      `insert into people (id, tenant_id, email, display_name, role)
       values ($1, $2, 'owner@chat.test', 'Owner', 'owner')`,
      [ownerId, tenantId],
    );
    await admin.query(
      `insert into canon_entries (id, tenant_id, domain, tier, owner_id, status, visibility, verify_interval, verified_at)
       values ($1, $2, 'pricing', 'operational', $3, 'active', '{"scope": "tenant"}', interval '60 days', now())`,
      [entryId, tenantId, ownerId],
    );
    await admin.query(
      `insert into canon_versions (id, tenant_id, entry_id, version_number, statement, created_by, status)
       values ($1, $2, $3, 1, 'The growth plan costs 1499 per month', $4, 'approved')`,
      [versionId, tenantId, entryId, ownerId],
    );
    await admin.query(
      `update canon_entries set current_version_id = $2 where id = $1`,
      [entryId, versionId],
    );

    process.env.DATABASE_URL = databaseUrl;
    process.env.INTERNAL_API_SECRET = secret;
    fake = new FakeChatLlm();
    const moduleRef = await Test.createTestingModule({ imports: [AppModule] })
      .overrideProvider(CHAT_LLM)
      .useValue(fake)
      .compile();
    app = configureApp(moduleRef.createNestApplication({ rawBody: true }));
    await app.init();
    http = app.getHttpServer() as Parameters<typeof request>[0];
  });

  afterAll(async () => {
    if (app) {
      await app.close();
    }
    for (const stmt of [
      `delete from chat_artifacts where tenant_id = $1`,
      `delete from chat_messages where tenant_id = $1`,
      `delete from chat_conversations where tenant_id = $1`,
      `delete from metering_events where tenant_id = $1`,
      `delete from drift_proposals where tenant_id = $1`,
      `delete from audit_log where tenant_id = $1`,
      `update canon_entries set current_version_id = null where tenant_id = $1`,
      `delete from canon_versions where tenant_id = $1`,
      `delete from canon_entries where tenant_id = $1`,
      `delete from people where tenant_id = $1`,
      `delete from tenants where id = $1`,
    ]) {
      await admin.query(stmt, [tenantId]);
    }
    await admin.end();
  });

  it('refuses requests without the internal secret', async () => {
    await request(http)
      .post('/chat/messages')
      .send({ content: 'hello' })
      .expect(401);
  });

  it('answers through the brain and stores the receipts', async () => {
    fake.queue.push({
      invoke: [
        { name: 'query_brain', input: { question: 'growth plan cost' } },
      ],
      text: 'The growth plan costs 1499 per month. Verified against the pricing canon.',
    });

    const response = await request(http)
      .post('/chat/messages')
      .set(headers)
      .send({ content: 'What does the growth plan cost?' })
      .expect(201);

    expect(response.body.message.content).toContain('1499');
    expect(response.body.message.citations[0].entry_id).toBe(entryId);

    const queried = fake.seen[0] as { trust: string };
    expect(queried.trust).toBe('canon_verified');

    const conversation = await request(http)
      .get(`/chat/conversations/${response.body.conversation_id}`)
      .set(headers)
      .expect(200);
    expect(conversation.body.messages).toHaveLength(2);
    expect(conversation.body.messages[0].role).toBe('user');
  });

  it('creates grounded deck artifacts linked to the message', async () => {
    fake.queue.push({
      invoke: [
        { name: 'query_brain', input: { question: 'pricing for the deck' } },
        {
          name: 'create_deck',
          input: {
            title: 'Enterprise pitch',
            slides: [
              {
                title: 'Pricing',
                bullets: ['Growth is 1499 per month'],
                entry_ids: [entryId],
              },
            ],
          },
        },
      ],
      text: 'Drafted the enterprise pitch deck from the pricing canon.',
    });

    const response = await request(http)
      .post('/chat/messages')
      .set(headers)
      .send({ content: 'Draft the enterprise pitch deck' })
      .expect(201);

    expect(response.body.artifacts).toHaveLength(1);
    const artifact = response.body.artifacts[0];
    expect(artifact.kind).toBe('deck');
    expect(artifact.content.slides[0].entry_ids).toEqual([entryId]);

    const linked = await admin.query<{ message_id: string | null }>(
      `select message_id from chat_artifacts where id = $1`,
      [artifact.id],
    );
    expect(linked.rows[0].message_id).toBe(response.body.message.id);
  });

  it('files corrections from chat into the drift pipeline', async () => {
    fake.queue.push({
      invoke: [
        {
          name: 'propose_update',
          input: {
            entry_id: entryId,
            statement: 'Growth plan is 1799 per month effective September',
          },
        },
      ],
      text: 'Filed the correction to the pricing owner for approval.',
    });

    await request(http)
      .post('/chat/messages')
      .set(headers)
      .send({ content: 'Pricing changed to 1799, fix the canon' })
      .expect(201);

    const proposal = await admin.query(
      `select kind, routed_to from drift_proposals
       where tenant_id = $1 and drafted_statement like '%1799%'`,
      [tenantId],
    );
    expect(proposal.rows[0]).toEqual({
      kind: 'contradiction',
      routed_to: ownerId,
    });
  });

  it('lists conversations for the person only', async () => {
    const list = await request(http)
      .get('/chat/conversations')
      .set(headers)
      .expect(200);
    expect(list.body.length).toBeGreaterThanOrEqual(3);
  });
});
