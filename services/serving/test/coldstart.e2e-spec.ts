import { randomUUID } from 'node:crypto';

import { Test } from '@nestjs/testing';
import { Pool } from 'pg';
import { z } from 'zod';

import { AppModule } from '../src/app.module';
import { ColdStartService } from '../src/coldstart/coldstart.service';
import type { ColdStartWire } from '../src/coldstart/schemas';
import { LLM_CLIENT } from '../src/drift/llm';
import type { LlmClient, LlmJsonRequest } from '../src/drift/llm';

const databaseUrl = process.env.TEST_DATABASE_URL;
const suite = databaseUrl ? describe : describe.skip;

class FakeLlm implements LlmClient {
  readonly enabled = true;
  queue: ColdStartWire[] = [];
  calls: LlmJsonRequest[] = [];

  completeJson<T>(request: LlmJsonRequest, schema: z.ZodType<T>): Promise<T> {
    this.calls.push(request);
    const scripted = this.queue.shift();
    if (!scripted) {
      throw new Error(`no scripted response for ${request.promptVersion}`);
    }
    return Promise.resolve(schema.parse(scripted));
  }
}

const vec = () => '[' + Array.from({ length: 1536 }, () => '0').join(',') + ']';

suite('cold start (e2e)', () => {
  let admin: Pool;
  let coldStart: ColdStartService;
  let fake: FakeLlm;
  let closeApp: () => Promise<void>;

  const tenantId = randomUUID();
  const adminId = randomUUID();
  const connectorId = randomUUID();
  const model = `coldstart-test-${randomUUID().slice(0, 8)}`;

  const insertChunk = async (content: string) => {
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
       select $1::uuid, $2, $3::uuid, e.occurred_at, 0, $4, $5::extensions.vector, $6,
              '{"scope": "tenant"}', 8, $1, array[$3::uuid], 'slack'
       from events e where e.id = $3::uuid`,
      [chunkId, tenantId, eventId, content, vec(), model],
    );
    return { eventId, chunkId };
  };

  const wireEntry = (
    overrides: Partial<ColdStartWire['entries'][number]> = {},
  ): ColdStartWire['entries'][number] => ({
    statement: 'Discounts above 15 percent require founder approval',
    domain: 'policy',
    tier: 'operational',
    attributes_json: '{"discount_ceiling_percent": 15}',
    confidence: 0.8,
    source_chunk_indexes: [0],
    topic: 'discount-approval',
    ...overrides,
  });

  beforeAll(async () => {
    admin = new Pool({ connectionString: databaseUrl });
    await admin.query(
      `insert into tenants (id, name, tier) values ($1, 'ColdStart E2E', 'growth')`,
      [tenantId],
    );
    await admin.query(
      `insert into people (id, tenant_id, email, display_name, role)
       values ($1, $2, 'admin@cs.test', 'Admin', 'admin')`,
      [adminId, tenantId],
    );
    await admin.query(
      `insert into connectors (id, tenant_id, source_type, status, config)
       values ($1, $2, 'slack', 'live', '{}')`,
      [connectorId, tenantId],
    );

    process.env.DATABASE_URL = databaseUrl;
    fake = new FakeLlm();
    const moduleRef = await Test.createTestingModule({ imports: [AppModule] })
      .overrideProvider(LLM_CLIENT)
      .useValue(fake)
      .compile();
    const app = moduleRef.createNestApplication();
    await app.init();
    coldStart = app.get(ColdStartService);
    closeApp = () => app.close();
  });

  afterAll(async () => {
    if (closeApp) {
      await closeApp();
    }
    for (const stmt of [
      `delete from drift_evidence where tenant_id = $1`,
      `delete from drift_proposals where tenant_id = $1`,
      `delete from cold_start_state where tenant_id = $1`,
      `delete from audit_log where tenant_id = $1`,
      `delete from canon_provenance where tenant_id = $1`,
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

  it('drafts entries from decision language chunks into the review pipeline', async () => {
    const { eventId } = await insertChunk(
      'U1: we decided discounts above 15 percent always need founder approval',
    );
    await insertChunk('U2: anyone up for lunch at the taco place');
    fake.queue.push({ entries: [wireEntry()] });

    const result = await coldStart.runOnce(tenantId);

    expect(result.chunksScanned).toBe(2);
    expect(result.candidates).toBe(1);
    expect(result.llmCalls).toBe(1);
    expect(result.entriesDrafted).toBe(1);

    const entry = await admin.query(
      `select ce.id, ce.status, ce.domain, ce.owner_id from canon_entries ce
       where ce.tenant_id = $1 and ce.domain = 'policy'`,
      [tenantId],
    );
    expect(entry.rows).toHaveLength(1);
    expect(entry.rows[0]).toMatchObject({ status: 'draft', owner_id: adminId });

    const provenance = await admin.query<{ event_id: string }>(
      `select cp.event_id from canon_provenance cp
       join canon_versions cv on cv.id = cp.version_id
       where cv.entry_id = $1`,
      [entry.rows[0].id],
    );
    expect(provenance.rows.map((r) => r.event_id)).toContain(eventId);

    const proposal = await admin.query(
      `select origin, kind, status, routed_to, pending_version_id from drift_proposals
       where tenant_id = $1 and entry_id = $2`,
      [tenantId, entry.rows[0].id],
    );
    expect(proposal.rows[0]).toMatchObject({
      origin: 'cold_start',
      kind: 'gap',
      status: 'pending',
      routed_to: adminId,
    });
    expect(proposal.rows[0].pending_version_id).not.toBeNull();
  });

  it('skips duplicates of existing statements across runs', async () => {
    await insertChunk(
      'U3: reminder we agreed discounts above 15 percent require founder approval',
    );
    fake.queue.push({
      entries: [
        wireEntry({
          statement:
            'Founder approval is required for discounts above 15 percent',
          topic: 'discount-approval-again',
        }),
      ],
    });

    const result = await coldStart.runOnce(tenantId);

    expect(result.skippedDuplicates).toBe(1);
    expect(result.entriesDrafted).toBe(0);
    const entries = await admin.query(
      `select count(*) as n from canon_entries where tenant_id = $1 and domain = 'policy'`,
      [tenantId],
    );
    expect(Number(entries.rows[0].n)).toBe(1);
  });

  it('escalates bedrock drafts and orders the review queue correctly', async () => {
    await insertChunk(
      'U4: we decided our positioning is provable trust, that is the strategy',
    );
    await insertChunk(
      'U5: pricing decision, growth stays at 1499 for the year',
    );
    fake.queue.push({
      entries: [
        wireEntry({
          statement:
            'Our positioning is provable trust for companies deploying AI',
          domain: 'positioning',
          tier: 'bedrock',
          attributes_json: '{"claim": "provable trust"}',
          confidence: 0.7,
          topic: 'positioning-trust',
        }),
        wireEntry({
          statement: 'The Growth plan stays at 1499 per month through the year',
          domain: 'pricing',
          attributes_json: '{"plan": "growth", "amount": 1499}',
          confidence: 0.9,
          topic: 'growth-price-hold',
        }),
      ],
    });

    const result = await coldStart.runOnce(tenantId);
    expect(result.entriesDrafted).toBe(2);

    const bedrock = await admin.query(
      `select strategic, escalated_to from drift_proposals
       where tenant_id = $1 and domain = 'positioning'`,
      [tenantId],
    );
    expect(bedrock.rows[0]).toMatchObject({
      strategic: true,
      escalated_to: adminId,
    });

    const queue = await coldStart.reviewQueue(tenantId);
    expect(queue).toHaveLength(3);
    expect(queue[0].tier).toBe('bedrock');
    expect(queue[1].domain).toBe('pricing');
  });

  it('respects the entry budget with a counted block', async () => {
    await admin.query(
      `update tenants set entry_budget = (
         select count(*) from canon_entries where tenant_id = $1 and status <> 'archived'
       ) where id = $1`,
      [tenantId],
    );
    await insertChunk(
      'U6: new policy decision, refunds are approved by finance only',
    );
    fake.queue.push({
      entries: [
        wireEntry({
          statement: 'Refunds are approved by finance only',
          topic: 'refund-approval',
          attributes_json: '{}',
        }),
      ],
    });

    const result = await coldStart.runOnce(tenantId);

    expect(result.budgetBlocked).toBe(1);
    expect(result.entriesDrafted).toBe(0);
  });

  it('advances the watermark so nothing rescans', async () => {
    const result = await coldStart.runOnce(tenantId);
    expect(result.chunksScanned).toBe(0);
    expect(fake.queue).toHaveLength(0);
  });

  it('infers org structure from communication patterns', async () => {
    await admin.query(`update tenants set entry_budget = 150 where id = $1`, [
      tenantId,
    ]);
    const insertEvent = (author: string, threadKey: string) =>
      admin.query(
        `insert into events (id, tenant_id, connector_id, source_type, external_id,
                             author_source_ref, thread_key, occurred_at, acl, payload_ref)
         values (gen_random_uuid(), $1, $2, 'slack', gen_random_uuid()::text, $3, $4, now(), '{"scope": "tenant"}', 'payloads/x')`,
        [tenantId, connectorId, author, threadKey],
      );
    for (let i = 0; i < 6; i++) {
      await insertEvent('slack:ALICE', `C-ENG:${i}`);
    }
    for (let i = 0; i < 3; i++) {
      await insertEvent('slack:BOB', `C-ENG:${i}`);
    }
    for (let i = 0; i < 5; i++) {
      await insertEvent('slack:CAROL', `C-SALES:${i}`);
    }

    fake.queue.push({
      entries: [
        wireEntry({
          statement:
            'Engineering is led by ALICE and handles product development',
          domain: 'org',
          attributes_json:
            '{"unit": "engineering", "lead": "slack:ALICE", "headcount": 2}',
          confidence: 0.6,
          source_chunk_indexes: [],
          topic: 'engineering-unit',
        }),
      ],
    });

    const result = await coldStart.inferOrg(tenantId);
    expect(result.alreadyInferred).toBe(false);
    expect(result.authorsAnalyzed).toBeGreaterThanOrEqual(3);
    expect(result.entriesDrafted).toBe(1);

    const orgCall = fake.calls.find((c) =>
      c.promptVersion.startsWith('coldstart-org'),
    );
    expect(orgCall).toBeDefined();
    expect(orgCall?.user).toContain('C-ENG');
    expect(orgCall?.user).toContain('slack:ALICE');

    const entry = await admin.query(
      `select ce.status from canon_entries ce
       where ce.tenant_id = $1 and ce.domain = 'org'`,
      [tenantId],
    );
    expect(entry.rows).toEqual([{ status: 'draft' }]);

    const state = await admin.query<{ org_inferred_at: Date | null }>(
      `select org_inferred_at from cold_start_state where tenant_id = $1`,
      [tenantId],
    );
    expect(state.rows[0].org_inferred_at).not.toBeNull();
  });

  it('runs org inference exactly once', async () => {
    const callsBefore = fake.calls.length;
    const result = await coldStart.inferOrg(tenantId);
    expect(result.alreadyInferred).toBe(true);
    expect(fake.calls).toHaveLength(callsBefore);
  });
});
