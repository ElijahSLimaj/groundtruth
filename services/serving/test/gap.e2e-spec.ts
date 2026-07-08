import { randomUUID } from 'node:crypto';

import { Test } from '@nestjs/testing';
import { Pool } from 'pg';
import { z } from 'zod';

import { AppModule } from '../src/app.module';
import { SERVING_CONFIG } from '../src/config';
import type { ServingConfig } from '../src/config';
import { DatabaseService } from '../src/database/database.service';
import { DriftService } from '../src/drift/drift.service';
import { GapService } from '../src/drift/gap.service';
import { LLM_CLIENT } from '../src/drift/llm';
import type { LlmClient, LlmJsonRequest } from '../src/drift/llm';
import { DisabledLlmClient } from '../src/drift/llm';
import type { GapTier2Result, GapTier3Wire } from '../src/drift/schemas';

const databaseUrl = process.env.TEST_DATABASE_URL;
const suite = databaseUrl ? describe : describe.skip;

class FakeLlm implements LlmClient {
  readonly enabled = true;
  gapTier2Queue: GapTier2Result[] = [];
  gapTier3Queue: GapTier3Wire[] = [];
  calls: LlmJsonRequest[] = [];

  completeJson<T>(request: LlmJsonRequest, schema: z.ZodType<T>): Promise<T> {
    this.calls.push(request);
    const scripted = request.promptVersion.startsWith('gap-tier2')
      ? this.gapTier2Queue.shift()
      : request.promptVersion.startsWith('gap-tier3')
        ? this.gapTier3Queue.shift()
        : undefined;
    if (!scripted) {
      throw new Error(`no scripted response for ${request.promptVersion}`);
    }
    return Promise.resolve(schema.parse(scripted));
  }
}

const vec = (pos: number) =>
  '[' +
  Array.from({ length: 1536 }, (_, i) => (i === pos ? '1' : '0')).join(',') +
  ']';

const worthy = (domain: string): GapTier2Result => ({
  canon_worthy: true,
  domain,
  confidence: 0.9,
});

const gapDraft = (statement: string): GapTier3Wire => ({
  drafted_statement: statement,
  drafted_attributes_json: '{"window_days": 30}',
  gap_description: 'refund policy keeps recurring with no coverage',
  supporting_excerpts: ['we honor refunds for 30 days'],
  confidence: 0.8,
});

suite('gap clustering (e2e)', () => {
  let admin: Pool;
  let drift: DriftService;
  let gap: GapService;
  let fake: FakeLlm;
  let closeApp: () => Promise<void>;
  let getService: <T>(token: unknown) => T;

  const tenantId = randomUUID();
  const adminId = randomUUID();
  const ownerId = randomUUID();
  const connectorId = randomUUID();
  const entryId = randomUUID();
  const versionId = randomUUID();
  const model = `gap-test-${randomUUID().slice(0, 8)}`;

  const insertChunk = async (position: number, author: string) => {
    const eventId = randomUUID();
    const chunkId = randomUUID();
    await admin.query(
      `insert into events (id, tenant_id, connector_id, source_type, external_id,
                           author_source_ref, thread_key, occurred_at, acl, payload_ref)
       values ($1::uuid, $2, $3, 'slack', $1, $4, $1, now(), '{"scope": "tenant"}', 'payloads/x')`,
      [eventId, tenantId, connectorId, author],
    );
    await admin.query(
      `insert into event_chunks (id, tenant_id, event_id, event_occurred_at, chunk_index,
                                 content, embedding, embedding_model, acl, token_count,
                                 window_key, member_event_ids, source_type)
       select $1::uuid, $2, $3::uuid, e.occurred_at, 0, $4, $5::extensions.vector, $6,
              '{"scope": "tenant"}', 8, $1, array[$3::uuid], 'slack'
       from events e where e.id = $3::uuid`,
      [
        chunkId,
        tenantId,
        eventId,
        `refund chatter from ${author}`,
        vec(position),
        model,
      ],
    );
    return chunkId;
  };

  const seedCluster = async (position: number, authors: string[]) => {
    for (const author of authors) {
      await insertChunk(position, author);
    }
    await drift.runOnce(tenantId);
  };

  const authors = (offset: number) =>
    Array.from({ length: 5 }, (_, i) => `slack:U${offset + i}`);

  beforeAll(async () => {
    admin = new Pool({ connectionString: databaseUrl });
    await admin.query(
      `insert into tenants (id, name, tier) values ($1, 'Gap E2E', 'growth')`,
      [tenantId],
    );
    await admin.query(
      `insert into people (id, tenant_id, email, display_name, role) values
       ($1, $3, 'admin@gap.test', 'Admin', 'admin'),
       ($2, $3, 'owner@gap.test', 'Owner', 'owner')`,
      [adminId, ownerId, tenantId],
    );
    await admin.query(
      `insert into connectors (id, tenant_id, source_type, status, config) values ($1, $2, 'slack', 'live', '{}')`,
      [connectorId, tenantId],
    );
    await admin.query(
      `insert into canon_entries (id, tenant_id, domain, tier, owner_id, status, visibility, verify_interval, verified_at)
       values ($1, $2, 'pricing', 'operational', $3, 'active', '{"scope": "tenant"}', interval '60 days', now())`,
      [entryId, tenantId, ownerId],
    );
    await admin.query(
      `insert into canon_versions (id, tenant_id, entry_id, version_number, statement, created_by, status)
       values ($1, $2, $3, 1, 'Growth plan is 1499 per month', $4, 'approved')`,
      [versionId, tenantId, entryId, ownerId],
    );
    await admin.query(
      `update canon_entries set current_version_id = $2 where id = $1`,
      [entryId, versionId],
    );
    await admin.query(
      `insert into canon_statement_embeddings (version_id, embedding_model, tenant_id, embedding)
       values ($1, $2, $3, $4::extensions.vector)`,
      [versionId, model, tenantId, vec(1)],
    );

    process.env.DATABASE_URL = databaseUrl;
    fake = new FakeLlm();
    const moduleRef = await Test.createTestingModule({ imports: [AppModule] })
      .overrideProvider(LLM_CLIENT)
      .useValue(fake)
      .compile();
    const app = moduleRef.createNestApplication();
    await app.init();
    drift = app.get(DriftService);
    gap = app.get(GapService);
    getService = (token) => app.get(token as never);
    closeApp = () => app.close();
  });

  afterAll(async () => {
    if (closeApp) {
      await closeApp();
    }
    for (const stmt of [
      `delete from unmatched_chunks where tenant_id = $1`,
      `delete from drift_evidence where tenant_id = $1`,
      `delete from drift_proposals where tenant_id = $1`,
      `delete from drift_state where tenant_id = $1`,
      `delete from drift_tuning where tenant_id = $1`,
      `delete from audit_log where tenant_id = $1`,
      `delete from canon_statement_embeddings where tenant_id = $1`,
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

  it('skips cleanly when no llm is configured', async () => {
    const disabled = new GapService(
      getService<DatabaseService>(DatabaseService),
      new DisabledLlmClient(),
      getService<ServingConfig>(SERVING_CONFIG),
    );
    const result = await disabled.runOnce(tenantId);
    expect(result.disabled).toBe(true);
  });

  it('buffers unmatched chunks from the drift scan without model calls', async () => {
    await seedCluster(40, authors(0));
    const buffered = await admin.query(
      `select count(*) as n from unmatched_chunks where tenant_id = $1`,
      [tenantId],
    );
    expect(Number(buffered.rows[0].n)).toBe(5);
    expect(fake.calls).toHaveLength(0);
  });

  it('proposes a gap entry from a diverse recurring cluster', async () => {
    fake.gapTier2Queue.push(worthy('pricing'));
    fake.gapTier3Queue.push(gapDraft('Refunds are honored within 30 days'));

    const result = await gap.runOnce(tenantId);
    expect(result.clusters).toBe(1);
    expect(result.proposalsCreated).toBe(1);

    const proposal = await admin.query<{
      id: string;
      kind: string;
      domain: string;
      routed_to: string;
      entry_id: string | null;
      drafted_statement: string;
    }>(
      `select id, kind, domain, routed_to, entry_id, drafted_statement
       from drift_proposals where tenant_id = $1 and kind = 'gap'`,
      [tenantId],
    );
    expect(proposal.rowCount).toBe(1);
    expect(proposal.rows[0].domain).toBe('pricing');
    expect(proposal.rows[0].routed_to).toBe(ownerId);
    expect(proposal.rows[0].entry_id).toBeNull();
    expect(proposal.rows[0].drafted_statement).toBe(
      'Refunds are honored within 30 days',
    );

    const evidence = await admin.query(
      `select count(*) as n from drift_evidence where proposal_id = $1`,
      [proposal.rows[0].id],
    );
    expect(Number(evidence.rows[0].n)).toBe(5);

    const buffered = await admin.query(
      `select count(*) as n from unmatched_chunks where tenant_id = $1`,
      [tenantId],
    );
    expect(Number(buffered.rows[0].n)).toBe(0);
  });

  it('attaches later clusters in the same domain as evidence', async () => {
    await seedCluster(41, authors(10));
    fake.gapTier2Queue.push(worthy('pricing'));

    const result = await gap.runOnce(tenantId);
    expect(result.proposalsCreated).toBe(0);
    expect(result.evidenceAttached).toBe(5);

    const evidence = await admin.query(
      `select count(*) as n from drift_evidence de
       join drift_proposals dp on dp.id = de.proposal_id
       where dp.tenant_id = $1 and dp.kind = 'gap'`,
      [tenantId],
    );
    expect(Number(evidence.rows[0].n)).toBe(10);
  });

  it('drops clusters judged not canon worthy', async () => {
    await seedCluster(42, authors(20));
    fake.gapTier2Queue.push({
      canon_worthy: false,
      domain: 'pricing',
      confidence: 0.9,
    });

    const result = await gap.runOnce(tenantId);
    expect(result.notCanonWorthy).toBe(1);
    expect(result.proposalsCreated).toBe(0);

    const buffered = await admin.query(
      `select count(*) as n from unmatched_chunks where tenant_id = $1`,
      [tenantId],
    );
    expect(Number(buffered.rows[0].n)).toBe(0);
  });

  it('leaves clusters below the diversity threshold buffered', async () => {
    await seedCluster(43, [
      'slack:SOLO',
      'slack:SOLO',
      'slack:SOLO',
      'slack:SOLO',
      'slack:SOLO',
    ]);

    const result = await gap.runOnce(tenantId);
    expect(result.clusters).toBe(0);

    const buffered = await admin.query(
      `select count(*) as n from unmatched_chunks where tenant_id = $1`,
      [tenantId],
    );
    expect(Number(buffered.rows[0].n)).toBe(5);
  });
});
