import { randomUUID } from 'node:crypto';

import { Test } from '@nestjs/testing';
import { Pool } from 'pg';
import { z } from 'zod';

import { AppModule } from '../src/app.module';
import { DriftService } from '../src/drift/drift.service';
import { LLM_CLIENT } from '../src/drift/llm';
import type { LlmClient, LlmJsonRequest } from '../src/drift/llm';
import { DisabledLlmClient } from '../src/drift/llm';
import type { Tier2Result, Tier3Wire } from '../src/drift/schemas';

const databaseUrl = process.env.TEST_DATABASE_URL;
const suite = databaseUrl ? describe : describe.skip;

class FakeLlm implements LlmClient {
  readonly enabled = true;
  tier2Queue: Tier2Result[] = [];
  tier3Queue: Tier3Wire[] = [];
  calls: LlmJsonRequest[] = [];

  completeJson<T>(request: LlmJsonRequest, schema: z.ZodType<T>): Promise<T> {
    this.calls.push(request);
    const scripted = request.promptVersion.startsWith('tier2')
      ? this.tier2Queue.shift()
      : this.tier3Queue.shift();
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

const tier3Draft = (statement: string): Tier3Wire => ({
  drafted_statement: statement,
  drafted_attributes_json: '{"amount": 1799}',
  contradiction_description: 'pricing changed in the sales channel',
  supporting_excerpts: ['1799 from August'],
  confidence: 0.85,
});

suite('drift engine (e2e)', () => {
  let admin: Pool;
  let drift: DriftService;
  let fake: FakeLlm;
  let closeApp: () => Promise<void>;

  const tenantId = randomUUID();
  const adminId = randomUUID();
  const ownerId = randomUUID();
  const connectorId = randomUUID();
  const pricingEntryId = randomUUID();
  const pricingVersionId = randomUUID();
  const bedrockEntryId = randomUUID();
  const bedrockVersionId = randomUUID();
  const model = `drift-test-${randomUUID().slice(0, 8)}`;

  const insertChunk = async (
    position: number,
    content: string,
    sourceType = 'slack',
  ) => {
    const eventId = randomUUID();
    const chunkId = randomUUID();
    await admin.query(
      `insert into events (id, tenant_id, connector_id, source_type, external_id,
                           author_source_ref, thread_key, occurred_at, acl, payload_ref)
       values ($1::uuid, $2, $3, $4, $1, 'slack:U1', $1, now(), '{"scope": "tenant"}', 'payloads/x')`,
      [eventId, tenantId, connectorId, sourceType],
    );
    await admin.query(
      `insert into event_chunks (id, tenant_id, event_id, event_occurred_at, chunk_index,
                                 content, embedding, embedding_model, acl, token_count,
                                 window_key, member_event_ids, source_type)
       select $1::uuid, $2, $3::uuid, e.occurred_at, 0, $4, $5::extensions.vector, $6,
              '{"scope": "tenant"}', 8, $1, array[$3::uuid], $7
       from events e where e.id = $3::uuid`,
      [chunkId, tenantId, eventId, content, vec(position), model, sourceType],
    );
    return { eventId, chunkId };
  };

  const insertEntry = async (
    entryId: string,
    versionId: string,
    tier: string,
    position: number,
    statement: string,
  ) => {
    await admin.query(
      `insert into canon_entries (id, tenant_id, domain, tier, owner_id, status, visibility, verify_interval, verified_at)
       values ($1, $2, 'pricing', $3, $4, 'active', '{"scope": "tenant"}', interval '60 days', now())`,
      [entryId, tenantId, tier, ownerId],
    );
    await admin.query(
      `insert into canon_versions (id, tenant_id, entry_id, version_number, statement, created_by, status)
       values ($1, $2, $3, 1, $4, $5, 'approved')`,
      [versionId, tenantId, entryId, statement, ownerId],
    );
    await admin.query(
      `update canon_entries set current_version_id = $2 where id = $1`,
      [entryId, versionId],
    );
    await admin.query(
      `insert into canon_statement_embeddings (version_id, embedding_model, tenant_id, embedding)
       values ($1, $2, $3, $4::extensions.vector)`,
      [versionId, model, tenantId, vec(position)],
    );
  };

  beforeAll(async () => {
    admin = new Pool({ connectionString: databaseUrl });
    await admin.query(
      `insert into tenants (id, name, tier) values ($1, 'Drift E2E', 'growth')`,
      [tenantId],
    );
    await admin.query(
      `insert into people (id, tenant_id, email, display_name, role) values
       ($1, $3, 'admin@drift.test', 'Admin', 'admin'),
       ($2, $3, 'owner@drift.test', 'Owner', 'owner')`,
      [adminId, ownerId, tenantId],
    );
    await admin.query(
      `insert into connectors (id, tenant_id, source_type, status, config) values ($1, $2, 'slack', 'live', '{}')`,
      [connectorId, tenantId],
    );
    await insertEntry(
      pricingEntryId,
      pricingVersionId,
      'operational',
      1,
      'Growth plan is 1499 per month',
    );
    await insertEntry(
      bedrockEntryId,
      bedrockVersionId,
      'bedrock',
      2,
      'We sell provable trust',
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
    closeApp = () => app.close();
  });

  afterAll(async () => {
    if (closeApp) {
      await closeApp();
    }
    for (const stmt of [
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
    const disabled = new DriftService(
      (drift as unknown as { db: never })['db'],
      new DisabledLlmClient(),
      (drift as unknown as { config: never })['config'],
    );
    const result = await disabled.runOnce(tenantId);
    expect(result.disabled).toBe(true);
    expect(result.chunksScanned).toBe(0);
  });

  it('creates a contradiction proposal with evidence through the full cascade', async () => {
    await insertChunk(1, 'the growth plan is 1799 per month starting August');
    fake.tier2Queue.push({
      relation: 'contradicts',
      confidence: 0.92,
      conflicting_field: 'amount',
    });
    fake.tier3Queue.push(
      tier3Draft('Growth plan is 1799 per month effective August'),
    );

    const result = await drift.runOnce(tenantId);

    expect(result.tier1Passed).toBe(1);
    expect(result.proposalsCreated).toBe(1);

    const proposal = await admin.query(
      `select kind, status, conflicting_field, routed_to, origin, strategic, drafted_statement
       from drift_proposals where tenant_id = $1 and entry_id = $2`,
      [tenantId, pricingEntryId],
    );
    expect(proposal.rows).toHaveLength(1);
    expect(proposal.rows[0]).toMatchObject({
      kind: 'contradiction',
      status: 'pending',
      conflicting_field: 'amount',
      routed_to: ownerId,
      origin: 'drift_engine',
      strategic: false,
    });
    expect(proposal.rows[0].drafted_statement).toContain('1799');

    const evidence = await admin.query(
      `select count(*) as n from drift_evidence de
       join drift_proposals dp on dp.id = de.proposal_id
       where dp.entry_id = $1`,
      [pricingEntryId],
    );
    expect(Number(evidence.rows[0].n)).toBeGreaterThanOrEqual(1);

    const audit = await admin.query(
      `select count(*) as n from audit_log where tenant_id = $1 and action = 'drift.proposal.created'`,
      [tenantId],
    );
    expect(Number(audit.rows[0].n)).toBe(1);
  });

  it('attaches new evidence to the open proposal instead of duplicating it', async () => {
    await insertChunk(1, 'sales quoted 1799 again on the acme call');
    fake.tier2Queue.push({
      relation: 'contradicts',
      confidence: 0.9,
      conflicting_field: 'amount',
    });

    const result = await drift.runOnce(tenantId);

    expect(result.proposalsCreated).toBe(0);
    expect(result.evidenceAttached).toBeGreaterThanOrEqual(1);
    const proposals = await admin.query(
      `select count(*) as n from drift_proposals where entry_id = $1 and status in ('pending', 'queued')`,
      [pricingEntryId],
    );
    expect(Number(proposals.rows[0].n)).toBe(1);
  });

  it('bumps last_referenced_at on confirms without proposing', async () => {
    await insertChunk(1, 'reminder that growth is 1499 per month');
    fake.tier2Queue.push({
      relation: 'confirms',
      confidence: 0.95,
      conflicting_field: null,
    });

    const result = await drift.runOnce(tenantId);

    expect(result.confirms).toBe(1);
    expect(result.proposalsCreated).toBe(0);
    const entry = await admin.query(
      `select last_referenced_at from canon_entries where id = $1`,
      [pricingEntryId],
    );
    expect(entry.rows[0].last_referenced_at).not.toBeNull();
  });

  it('drops low similarity chunks before any model call', async () => {
    await insertChunk(40, 'completely unrelated lunch plans');
    const callsBefore = fake.calls.length;

    const result = await drift.runOnce(tenantId);

    expect(result.chunksScanned).toBe(1);
    expect(result.tier1Passed).toBe(0);
    expect(fake.calls.length).toBe(callsBefore);
  });

  it('drops classifications below the tier2 confidence gate', async () => {
    await insertChunk(1, 'maybe the growth price changes someday');
    fake.tier2Queue.push({
      relation: 'contradicts',
      confidence: 0.4,
      conflicting_field: 'amount',
    });

    const result = await drift.runOnce(tenantId);

    expect(result.unrelated).toBe(1);
    expect(result.tier3Drafted).toBe(0);
  });

  it('escalates bedrock contradictions to the admin as strategic', async () => {
    await insertChunk(2, 'we are pivoting away from trust as the pitch');
    fake.tier2Queue.push({
      relation: 'contradicts',
      confidence: 0.9,
      conflicting_field: null,
    });
    fake.tier3Queue.push(tier3Draft('We sell speed, not trust'));

    const result = await drift.runOnce(tenantId);

    expect(result.proposalsCreated).toBe(1);
    const proposal = await admin.query(
      `select strategic, escalated_to, routed_to from drift_proposals where entry_id = $1`,
      [bedrockEntryId],
    );
    expect(proposal.rows[0]).toMatchObject({
      strategic: true,
      escalated_to: adminId,
      routed_to: ownerId,
    });
  });

  it('queues proposals beyond the owner weekly budget', async () => {
    await admin.query(
      `insert into drift_tuning (tenant_id, params) values ($1, '{"owner_weekly_budget": 1}')
       on conflict (tenant_id) do update set params = excluded.params`,
      [tenantId],
    );
    await insertChunk(1, 'discounts now capped at 25 percent apparently');
    fake.tier2Queue.push({
      relation: 'extends',
      confidence: 0.88,
      conflicting_field: 'discount_ceiling_percent',
    });
    fake.tier3Queue.push(
      tier3Draft(
        'Growth plan is 1499 per month, discounts capped at 25 percent',
      ),
    );

    const result = await drift.runOnce(tenantId);

    expect(result.queuedByBudget).toBe(1);
    const proposal = await admin.query(
      `select kind, status from drift_proposals
       where entry_id = $1 and conflicting_field = 'discount_ceiling_percent'`,
      [pricingEntryId],
    );
    expect(proposal.rows[0]).toMatchObject({
      kind: 'extension',
      status: 'queued',
    });
  });

  it('advances the watermark so nothing is rescanned', async () => {
    const result = await drift.runOnce(tenantId);
    expect(result.chunksScanned).toBe(0);
  });
});
