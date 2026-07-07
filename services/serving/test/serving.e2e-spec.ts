import { createHash, randomUUID } from 'node:crypto';

import { INestApplication } from '@nestjs/common';
import { Test } from '@nestjs/testing';
import { Pool } from 'pg';
import request from 'supertest';

import { AppModule } from '../src/app.module';
import { configureApp } from '../src/configure-app';

const databaseUrl = process.env.TEST_DATABASE_URL;
const suite = databaseUrl ? describe : describe.skip;

suite('serving api (e2e)', () => {
  let app: INestApplication;
  let admin: Pool;
  let httpServer: Parameters<typeof request>[0];

  const tenantId = randomUUID();
  const ownerId = randomUUID();
  const agentId = randomUUID();
  const outsiderId = randomUUID();
  const pricingEntryId = randomUUID();
  const pricingVersionId = randomUUID();
  const decayedEntryId = randomUUID();
  const decayedVersionId = randomUUID();
  const privateEntryId = randomUUID();
  const privateVersionId = randomUUID();
  const conflictProposalId = randomUUID();

  const agentKey = `cbk_test_${randomUUID()}`;
  const scopedKey = `cbk_scoped_${randomUUID()}`;
  const tinyKey = `cbk_tiny_${randomUUID()}`;
  const hash = (key: string) => createHash('sha256').update(key).digest('hex');
  const auth = (key: string) => ({ Authorization: `Bearer ${key}` });

  beforeAll(async () => {
    admin = new Pool({ connectionString: databaseUrl });

    await admin.query(
      `insert into tenants (id, name, tier) values ($1, 'E2E Tenant', 'growth')`,
      [tenantId],
    );
    await admin.query(
      `insert into people (id, tenant_id, email, display_name, role) values
       ($1, $4, 'owner@e2e.test', 'Owner', 'owner'),
       ($2, $4, 'agent@e2e.test', 'Agent', 'agent'),
       ($3, $4, 'outsider@e2e.test', 'Outsider', 'member')`,
      [ownerId, agentId, outsiderId, tenantId],
    );

    const insertEntry = async (
      entryId: string,
      versionId: string,
      domain: string,
      statement: string,
      status: string,
      verifiedAt: string,
      visibility: string,
    ) => {
      await admin.query(
        `insert into canon_entries (id, tenant_id, domain, tier, owner_id, status, visibility, verify_interval, verified_at)
         values ($1, $2, $3, 'operational', $4, $5, $6, interval '60 days', $7::timestamptz)`,
        [entryId, tenantId, domain, ownerId, status, visibility, verifiedAt],
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
        `insert into approvals (tenant_id, version_id, approver_id, decision) values ($1, $2, $3, 'approved')`,
        [tenantId, versionId, ownerId],
      );
    };

    await insertEntry(
      pricingEntryId,
      pricingVersionId,
      'pricing',
      'The growth plan costs 1499 per month with annual billing discounts',
      'active',
      new Date().toISOString(),
      '{"scope": "tenant"}',
    );
    await insertEntry(
      decayedEntryId,
      decayedVersionId,
      'process',
      'Refund requests are handled by support within five business days',
      'decayed',
      new Date(Date.now() - 200 * 86400_000).toISOString(),
      '{"scope": "tenant"}',
    );
    await insertEntry(
      privateEntryId,
      privateVersionId,
      'org',
      'Confidential reorg planning for the platform unit',
      'active',
      new Date().toISOString(),
      `{"scope": "principals", "principals": ["person:${outsiderId}"]}`,
    );

    await admin.query(
      `insert into drift_proposals (id, tenant_id, entry_id, kind, drafted_statement, confidence, routed_to)
       values ($1, $2, $3, 'contradiction', 'Growth pricing changed to 1799 in a founder message', 0.9, $4)`,
      [conflictProposalId, tenantId, pricingEntryId, ownerId],
    );

    await admin.query(
      `insert into api_keys (tenant_id, person_id, key_hash, name, allowed_domains, rate_tier) values
       ($1, $2, $3, 'agent key', null, 'standard'),
       ($1, $2, $4, 'scoped key', array['org'], 'standard'),
       ($1, $2, $5, 'tiny key', null, 'minimal')`,
      [tenantId, agentId, hash(agentKey), hash(scopedKey), hash(tinyKey)],
    );

    process.env.DATABASE_URL = databaseUrl;
    const moduleRef = await Test.createTestingModule({
      imports: [AppModule],
    }).compile();
    app = configureApp(moduleRef.createNestApplication());
    await app.init();
    httpServer = app.getHttpServer() as Parameters<typeof request>[0];
  });

  afterAll(async () => {
    if (app) {
      await app.close();
    }
    await admin.query(
      `update canon_entries set current_version_id = null where tenant_id = $1`,
      [tenantId],
    );
    for (const stmt of [
      `delete from metering_events where tenant_id = $1`,
      `delete from audit_log where tenant_id = $1`,
      `delete from api_keys where tenant_id = $1`,
      `delete from drift_proposals where tenant_id = $1`,
      `delete from approvals where tenant_id = $1`,
      `delete from canon_provenance where tenant_id = $1`,
      `delete from canon_versions where tenant_id = $1`,
      `delete from canon_entries where tenant_id = $1`,
      `delete from people where tenant_id = $1`,
      `delete from tenants where id = $1`,
    ]) {
      await admin.query(stmt, [tenantId]);
    }
    await admin.end();
  });

  it('serves the unauthenticated health route', async () => {
    await request(httpServer).get('/').expect(200);
  });

  it('rejects requests without a key', async () => {
    await request(httpServer)
      .post('/tools/query')
      .send({ question: 'what does the growth plan cost?' })
      .expect(401);
  });

  it('rejects unknown keys', async () => {
    await request(httpServer)
      .post('/tools/query')
      .set({ Authorization: 'Bearer cbk_nope' })
      .send({ question: 'anything' })
      .expect(401);
  });

  it('answers a canon question with a receipt', async () => {
    const res = await request(httpServer)
      .post('/tools/query')
      .set(auth(agentKey))
      .send({ question: 'growth plan cost', domains: ['pricing'] })
      .expect(201);

    expect(res.body.trust).toBe('canon_verified');
    expect(res.body.answer).toContain('1499 per month');
    expect(res.body.citations).toHaveLength(1);
    const citation = res.body.citations[0];
    expect(citation.entry_id).toBe(pricingEntryId);
    expect(citation.version).toBe(1);
    expect(citation.approver).toBe(`person:${ownerId}`);
    expect(citation.verified_at).toMatch(/^\d{4}-\d{2}-\d{2}$/);
    expect(res.body.conflicts).toHaveLength(1);
    expect(res.body.conflicts[0].proposal_id).toBe(conflictProposalId);
    expect(res.body.freshness.decayed_entries_used).toBe(0);
  });

  it('labels decayed canon as canon_stale', async () => {
    const res = await request(httpServer)
      .post('/tools/query')
      .set(auth(agentKey))
      .send({ question: 'refund requests support' })
      .expect(201);

    expect(res.body.trust).toBe('canon_stale');
    expect(res.body.freshness.decayed_entries_used).toBe(1);
  });

  it('treats no coverage as a first class answer', async () => {
    const res = await request(httpServer)
      .post('/tools/query')
      .set(auth(agentKey))
      .send({ question: 'xylophone quarterly maintenance cadence' })
      .expect(201);

    expect(res.body.trust).toBe('no_coverage');
    expect(res.body.citations).toHaveLength(0);
    expect(res.body.answer).toBe('No governed knowledge covers this question.');
  });

  it('never cites entries outside the caller visibility', async () => {
    const res = await request(httpServer)
      .post('/tools/query')
      .set(auth(agentKey))
      .send({ question: 'confidential reorg planning platform' })
      .expect(201);

    expect(res.body.trust).toBe('no_coverage');
  });

  it('rejects queries outside the key domain scope', async () => {
    await request(httpServer)
      .post('/tools/query')
      .set(auth(scopedKey))
      .send({ question: 'growth plan cost', domains: ['pricing'] })
      .expect(403);
  });

  it('rejects malformed requests with 422', async () => {
    await request(httpServer)
      .post('/tools/query')
      .set(auth(agentKey))
      .send({ question: '', max_citations: 0 })
      .expect(422);
  });

  it('returns a full entry with history and relations', async () => {
    const res = await request(httpServer)
      .get(`/tools/entries/${pricingEntryId}`)
      .set(auth(agentKey))
      .expect(200);

    expect(res.body.entry.id).toBe(pricingEntryId);
    expect(res.body.entry.statement).toContain('1499');
    expect(res.body.versions).toHaveLength(1);
    expect(Array.isArray(res.body.provenance)).toBe(true);
    expect(Array.isArray(res.body.relations)).toBe(true);
  });

  it('hides invisible entries behind 404', async () => {
    await request(httpServer)
      .get(`/tools/entries/${privateEntryId}`)
      .set(auth(agentKey))
      .expect(404);
    await request(httpServer)
      .get(`/tools/entries/${randomUUID()}`)
      .set(auth(agentKey))
      .expect(404);
  });

  it('lists open conflicts with domain filtering', async () => {
    const all = await request(httpServer)
      .get('/tools/conflicts')
      .set(auth(agentKey))
      .expect(200);
    expect(all.body).toHaveLength(1);
    expect(all.body[0].proposal_id).toBe(conflictProposalId);

    const filtered = await request(httpServer)
      .get('/tools/conflicts?domain=org')
      .set(auth(agentKey))
      .expect(200);
    expect(filtered.body).toHaveLength(0);
  });

  it('routes agent proposals into the owner pipeline', async () => {
    const res = await request(httpServer)
      .post('/tools/proposals')
      .set(auth(agentKey))
      .send({
        entry_id: pricingEntryId,
        statement: 'Growth plan is 1799 per month effective August',
      })
      .expect(201);

    expect(res.body.kind).toBe('contradiction');
    expect(res.body.routed_to).toBe(ownerId);

    const row = await admin.query(
      `select origin, status from drift_proposals where id = $1`,
      [res.body.proposal_id],
    );
    expect(row.rows[0]).toEqual({ origin: 'agent', status: 'pending' });
  });

  it('requires entry_id or domain on proposals', async () => {
    await request(httpServer)
      .post('/tools/proposals')
      .set(auth(agentKey))
      .send({ statement: 'orphaned statement' })
      .expect(422);
  });

  it('rate limits per key with retry-after', async () => {
    await request(httpServer)
      .get('/tools/conflicts')
      .set(auth(tinyKey))
      .expect(200);
    await request(httpServer)
      .get('/tools/conflicts')
      .set(auth(tinyKey))
      .expect(200);
    const limited = await request(httpServer)
      .get('/tools/conflicts')
      .set(auth(tinyKey))
      .expect(429);
    expect(Number(limited.headers['retry-after'])).toBeGreaterThan(0);
  });

  it('audits every served call with tool and trust', async () => {
    const rows = await admin.query(
      `select detail from audit_log
       where tenant_id = $1 and action = 'serving.query'
       order by occurred_at desc`,
      [tenantId],
    );
    expect(rows.rows.length).toBeGreaterThan(0);
    const detail = rows.rows[0].detail as { tool: string; trust: string };
    expect(detail.tool).toBe('query');
    expect(detail.trust).toBeDefined();
  });
});
