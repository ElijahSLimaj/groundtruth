import { createHash, randomUUID } from 'node:crypto';

import { INestApplication } from '@nestjs/common';
import { Test } from '@nestjs/testing';
import { Pool } from 'pg';
import request from 'supertest';

import { AppModule } from '../src/app.module';
import { configureApp } from '../src/configure-app';
import { SchedulerService } from '../src/scheduler/scheduler.service';

const databaseUrl = process.env.TEST_DATABASE_URL;
const suite = databaseUrl ? describe : describe.skip;

suite('admin key management and metering (e2e)', () => {
  let app: INestApplication;
  let admin: Pool;
  let httpServer: Parameters<typeof request>[0];

  const tenantId = randomUUID();
  const adminId = randomUUID();
  const agentId = randomUUID();
  const adminKey = `cbk_admin_${randomUUID()}`;
  const agentKey = `cbk_agent_${randomUUID()}`;
  const auth = (key: string) => ({ Authorization: `Bearer ${key}` });
  const hash = (key: string) => createHash('sha256').update(key).digest('hex');

  beforeAll(async () => {
    admin = new Pool({ connectionString: databaseUrl });
    await admin.query(
      `insert into tenants (id, name, tier) values ($1, 'Admin E2E', 'growth')`,
      [tenantId],
    );
    await admin.query(
      `insert into people (id, tenant_id, email, display_name, role) values
       ($1, $3, 'admin@adm.test', 'Admin', 'admin'),
       ($2, $3, 'agent@adm.test', 'Agent', 'agent')`,
      [adminId, agentId, tenantId],
    );
    await admin.query(
      `insert into api_keys (tenant_id, person_id, key_hash, name) values
       ($1, $2, $3, 'admin key'),
       ($1, $4, $5, 'agent key')`,
      [tenantId, adminId, hash(adminKey), agentId, hash(agentKey)],
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
    for (const stmt of [
      `delete from metering_events where tenant_id = $1`,
      `delete from audit_log where tenant_id = $1`,
      `delete from api_keys where tenant_id = $1`,
      `delete from people where tenant_id = $1`,
      `delete from tenants where id = $1`,
    ]) {
      await admin.query(stmt, [tenantId]);
    }
    await admin.end();
  });

  it('lets an admin mint a scoped key that immediately works', async () => {
    const created = await request(httpServer)
      .post('/admin/keys')
      .set(auth(adminKey))
      .send({
        person_id: agentId,
        name: 'support bot',
        allowed_domains: ['pricing'],
        rate_tier: 'standard',
      })
      .expect(201);

    expect(created.body.key).toMatch(/^cbk_/);
    await request(httpServer)
      .get('/tools/conflicts')
      .set(auth(created.body.key as string))
      .expect(200);
  });

  it('refuses key management to non admins without confirming anything', async () => {
    await request(httpServer)
      .post('/admin/keys')
      .set(auth(agentKey))
      .send({ person_id: agentId, name: 'sneaky' })
      .expect(403);
  });

  it('revokes keys and kills their access', async () => {
    const created = await request(httpServer)
      .post('/admin/keys')
      .set(auth(adminKey))
      .send({ person_id: agentId, name: 'short lived' })
      .expect(201);

    await request(httpServer)
      .delete(`/admin/keys/${created.body.id}`)
      .set(auth(adminKey))
      .expect(200);

    await request(httpServer)
      .get('/tools/conflicts')
      .set(auth(created.body.key as string))
      .expect(401);
  });

  it('meters every authenticated call by tool', async () => {
    await request(httpServer)
      .get('/tools/conflicts')
      .set(auth(agentKey))
      .expect(200);

    const rows = await admin.query<{ tool: string; calls: string }>(
      `select tool, count(*) as calls from metering_events
       where tenant_id = $1 group by tool order by tool`,
      [tenantId],
    );
    const tools = rows.rows.map((r) => r.tool);
    expect(tools).toContain('/tools/conflicts');
    expect(tools).toContain('/admin/keys');
  });

  it('rate limits from the shared bucket and never meters a refused call', async () => {
    const created = await request(httpServer)
      .post('/admin/keys')
      .set(auth(adminKey))
      .send({ person_id: agentId, name: 'minimal tier', rate_tier: 'minimal' })
      .expect(201);
    const key = created.body.key as string;

    await request(httpServer)
      .get('/tools/conflicts')
      .set(auth(key))
      .expect(200);
    await request(httpServer)
      .get('/tools/conflicts')
      .set(auth(key))
      .expect(200);

    const refused = await request(httpServer)
      .get('/tools/conflicts')
      .set(auth(key))
      .expect(429);
    expect(Number(refused.headers['retry-after'])).toBeGreaterThan(0);

    const metered = await admin.query<{ n: string }>(
      `select count(*) as n from metering_events
       where tenant_id = $1 and api_key_id = $2`,
      [tenantId, created.body.id],
    );
    expect(Number(metered.rows[0].n)).toBe(2);
  });

  it('enumerates tenants for the scheduler without tenant context', async () => {
    const scheduler = app.get(SchedulerService);
    const tenants = await scheduler.tenants();
    expect(tenants).toContain(tenantId);
  });
});
