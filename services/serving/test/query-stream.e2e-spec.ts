import { createHash, randomUUID } from 'node:crypto';

import { INestApplication } from '@nestjs/common';
import { Test } from '@nestjs/testing';
import { Pool } from 'pg';
import request from 'supertest';
import { z } from 'zod';

import { AppModule } from '../src/app.module';
import { configureApp } from '../src/configure-app';
import { EMBEDDER } from '../src/canon/embedder';
import type { Embedder } from '../src/canon/embedder';
import { LLM_CLIENT } from '../src/drift/llm';
import type { LlmClient, LlmJsonRequest } from '../src/drift/llm';

const databaseUrl = process.env.TEST_DATABASE_URL;
const suite = databaseUrl ? describe : describe.skip;

class FakeSynthesisLlm implements LlmClient {
  readonly enabled = true;
  lastRequest: LlmJsonRequest | null = null;
  failNext = false;

  completeJson<T>(request: LlmJsonRequest, schema: z.ZodType<T>): Promise<T> {
    this.lastRequest = request;
    if (this.failNext) {
      this.failNext = false;
      return Promise.reject(new Error('synthesis unavailable'));
    }
    return Promise.resolve(schema.parse({ answer: 'synthesized answer' }));
  }
}

class BasisEmbedder implements Embedder {
  readonly modelId: string;
  shouldThrow = false;

  constructor(modelId: string) {
    this.modelId = modelId;
  }

  embed(): Promise<number[]> {
    if (this.shouldThrow) {
      return Promise.reject(new Error('embedding provider down'));
    }
    return Promise.resolve(
      Array.from({ length: 1536 }, (_, i) => (i === 0 ? 1 : 0)),
    );
  }
}

suite('query stream fallback and synthesis (e2e)', () => {
  let app: INestApplication;
  let admin: Pool;
  let httpServer: Parameters<typeof request>[0];
  let llm: FakeSynthesisLlm;
  let embedder: BasisEmbedder;

  const tenantId = randomUUID();
  const ownerId = randomUUID();
  const agentId = randomUUID();
  const connectorId = randomUUID();
  const pricingEntryId = randomUUID();
  const pricingVersionId = randomUUID();
  const model = `query-test-${randomUUID().slice(0, 8)}`;
  const agentKey = `cbk_query_${randomUUID()}`;
  const auth = { Authorization: `Bearer ${agentKey}` };

  beforeAll(async () => {
    admin = new Pool({ connectionString: databaseUrl });
    await admin.query(
      `insert into tenants (id, name, tier) values ($1, 'Query E2E', 'growth')`,
      [tenantId],
    );
    await admin.query(
      `insert into people (id, tenant_id, email, display_name, role) values
       ($1, $3, 'owner@q.test', 'Owner', 'owner'),
       ($2, $3, 'agent@q.test', 'Agent', 'agent')`,
      [ownerId, agentId, tenantId],
    );
    await admin.query(
      `insert into connectors (id, tenant_id, source_type, status, config)
       values ($1, $2, 'slack', 'live', '{}')`,
      [connectorId, tenantId],
    );
    await admin.query(
      `insert into api_keys (tenant_id, person_id, key_hash, name) values ($1, $2, $3, 'query key')`,
      [tenantId, agentId, createHash('sha256').update(agentKey).digest('hex')],
    );

    await admin.query(
      `insert into canon_entries (id, tenant_id, domain, tier, owner_id, status, visibility, verify_interval, verified_at)
       values ($1, $2, 'pricing', 'operational', $3, 'active', '{"scope": "tenant"}', interval '60 days', now())`,
      [pricingEntryId, tenantId, ownerId],
    );
    await admin.query(
      `insert into canon_versions (id, tenant_id, entry_id, version_number, statement, created_by, status)
       values ($1, $2, $3, 1, 'The growth plan costs 1499 per month', $4, 'approved')`,
      [pricingVersionId, tenantId, pricingEntryId, ownerId],
    );
    await admin.query(
      `update canon_entries set current_version_id = $2 where id = $1`,
      [pricingEntryId, pricingVersionId],
    );

    const eventId = randomUUID();
    const chunkId = randomUUID();
    await admin.query(
      `insert into events (id, tenant_id, connector_id, source_type, external_id,
                           author_source_ref, thread_key, occurred_at, acl, payload_ref)
       values ($1::uuid, $2, $3, 'slack', $1, 'slack:U1', $1, now(), '{"scope": "tenant"}', 'payloads/x')`,
      [eventId, tenantId, connectorId],
    );
    await admin.query(
      `insert into event_chunks (id, tenant_id, event_id, event_occurred_at, chunk_index,
                                 content, embedding, embedding_model, acl, token_count,
                                 window_key, member_event_ids, source_type)
       select $1::uuid, $2, $3::uuid, e.occurred_at, 0,
              'U1: the offsite is confirmed for the second week of september in lisbon',
              $4::extensions.vector, $5, '{"scope": "tenant"}', 12, $1, array[$3::uuid], 'slack'
       from events e where e.id = $3::uuid`,
      [
        chunkId,
        tenantId,
        eventId,
        '[' +
          Array.from({ length: 1536 }, (_, i) => (i === 0 ? '1' : '0')).join(
            ',',
          ) +
          ']',
        model,
      ],
    );

    process.env.DATABASE_URL = databaseUrl;
    llm = new FakeSynthesisLlm();
    embedder = new BasisEmbedder(model);
    const moduleRef = await Test.createTestingModule({ imports: [AppModule] })
      .overrideProvider(LLM_CLIENT)
      .useValue(llm)
      .overrideProvider(EMBEDDER)
      .useValue(embedder)
      .compile();
    app = configureApp(moduleRef.createNestApplication());
    await app.init();
    httpServer = app.getHttpServer() as Parameters<typeof request>[0];
  });

  afterAll(async () => {
    if (app) {
      await app.close();
    }
    for (const stmt of [
      `delete from audit_log where tenant_id = $1`,
      `delete from metering_events where tenant_id = $1`,
      `delete from api_keys where tenant_id = $1`,
      `update canon_entries set current_version_id = null where tenant_id = $1`,
      `delete from canon_versions where tenant_id = $1`,
      `delete from canon_entries where tenant_id = $1`,
      `delete from event_chunks where tenant_id = $1`,
      `delete from events where tenant_id = $1`,
      `delete from connectors where tenant_id = $1`,
      `delete from people where tenant_id = $1`,
      `delete from tenants where id = $1`,
    ]) {
      await admin.query(stmt, [tenantId]);
    }
    await admin.end();
  });

  it('synthesizes canon answers when a model is available', async () => {
    const res = await request(httpServer)
      .post('/tools/query')
      .set(auth)
      .send({ question: 'growth plan cost' })
      .expect(201);

    expect(res.body.trust).toBe('canon_verified');
    expect(res.body.answer).toBe('synthesized answer');
    expect(llm.lastRequest?.promptVersion).toBe('synthesis-v1');
    expect(llm.lastRequest?.user).toContain('1499 per month');
  });

  it('falls back to stream retrieval labeled stream_only', async () => {
    const res = await request(httpServer)
      .post('/tools/query')
      .set(auth)
      .send({ question: 'when is the offsite in lisbon', include_stream: true })
      .expect(201);

    expect(res.body.trust).toBe('stream_only');
    expect(res.body.citations).toHaveLength(1);
    expect(res.body.citations[0].type).toBe('stream');
    expect(res.body.citations[0].excerpt).toContain('september');
    expect(llm.lastRequest?.user).toContain('september');
  });

  it('does not touch the stream when canon answers', async () => {
    const res = await request(httpServer)
      .post('/tools/query')
      .set(auth)
      .send({ question: 'growth plan cost', include_stream: true })
      .expect(201);

    expect(res.body.trust).toBe('canon_verified');
    const citations = res.body.citations as { type: string }[];
    expect(citations.every((c) => c.type === 'canon')).toBe(true);
  });

  it('keeps include_stream inert without the flag', async () => {
    const res = await request(httpServer)
      .post('/tools/query')
      .set(auth)
      .send({ question: 'when is the offsite in lisbon' })
      .expect(201);

    expect(res.body.trust).toBe('no_coverage');
  });

  it('serves degraded but valid answers when synthesis fails', async () => {
    llm.failNext = true;
    const res = await request(httpServer)
      .post('/tools/query')
      .set(auth)
      .send({ question: 'growth plan cost' })
      .expect(201);

    expect(res.body.trust).toBe('canon_verified');
    expect(res.body.answer).toContain('1499 per month');
  });

  it('labels retrieval degradation when the stream path is down', async () => {
    embedder.shouldThrow = true;
    const res = await request(httpServer)
      .post('/tools/query')
      .set(auth)
      .send({ question: 'when is the offsite in lisbon', include_stream: true })
      .expect(201);
    embedder.shouldThrow = false;

    expect(res.body.trust).toBe('no_coverage');
    expect(res.body.retrieval_degraded).toBe(true);
  });
});
