import { createHash, randomUUID } from 'node:crypto';

import { Client } from '@modelcontextprotocol/sdk/client/index.js';
import { StreamableHTTPClientTransport } from '@modelcontextprotocol/sdk/client/streamableHttp.js';
import { INestApplication } from '@nestjs/common';
import { Test } from '@nestjs/testing';
import { Pool } from 'pg';

import { AppModule } from '../src/app.module';
import { configureApp } from '../src/configure-app';

const databaseUrl = process.env.TEST_DATABASE_URL;
const suite = databaseUrl ? describe : describe.skip;

suite('mcp interface (e2e)', () => {
  let app: INestApplication;
  let admin: Pool;
  let client: Client;
  let baseUrl: string;

  const tenantId = randomUUID();
  const ownerId = randomUUID();
  const agentId = randomUUID();
  const entryId = randomUUID();
  const versionId = randomUUID();
  const agentKey = `cbk_mcp_${randomUUID()}`;

  beforeAll(async () => {
    admin = new Pool({ connectionString: databaseUrl });
    await admin.query(
      `insert into tenants (id, name, tier) values ($1, 'MCP E2E', 'growth')`,
      [tenantId],
    );
    await admin.query(
      `insert into people (id, tenant_id, email, display_name, role) values
       ($1, $3, 'owner@mcp.test', 'Owner', 'owner'),
       ($2, $3, 'agent@mcp.test', 'Agent', 'agent')`,
      [ownerId, agentId, tenantId],
    );
    await admin.query(
      `insert into api_keys (tenant_id, person_id, key_hash, name) values ($1, $2, $3, 'mcp key')`,
      [tenantId, agentId, createHash('sha256').update(agentKey).digest('hex')],
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
    const moduleRef = await Test.createTestingModule({
      imports: [AppModule],
    }).compile();
    app = configureApp(moduleRef.createNestApplication({ rawBody: true }));
    await app.listen(0);
    baseUrl = (await app.getUrl()).replace('[::1]', '127.0.0.1');

    client = new Client({ name: 'e2e', version: '1.0.0' });
    await client.connect(
      new StreamableHTTPClientTransport(new URL(`${baseUrl}/mcp`), {
        requestInit: { headers: { Authorization: `Bearer ${agentKey}` } },
      }),
    );
  });

  afterAll(async () => {
    if (client) {
      await client.close();
    }
    if (app) {
      await app.close();
    }
    for (const stmt of [
      `delete from metering_events where tenant_id = $1`,
      `delete from drift_proposals where tenant_id = $1`,
      `delete from audit_log where tenant_id = $1`,
      `delete from api_keys where tenant_id = $1`,
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

  it('lists the four spec tools', async () => {
    const tools = await client.listTools();
    const names = tools.tools.map((t) => t.name).sort();
    expect(names).toEqual([
      'get_entry',
      'list_conflicts',
      'propose_update',
      'query',
    ]);
  });

  it('answers query with the full contract over mcp', async () => {
    const result = await client.callTool({
      name: 'query',
      arguments: { question: 'growth plan cost' },
    });
    const content = result.content as { type: string; text: string }[];
    const body = JSON.parse(content[0].text) as {
      trust: string;
      answer: string;
      citations: { entry_id: string }[];
    };
    expect(body.trust).toBe('canon_verified');
    expect(body.answer).toContain('1499');
    expect(body.citations[0].entry_id).toBe(entryId);
  });

  it('submits proposals into the owner pipeline over mcp', async () => {
    const result = await client.callTool({
      name: 'propose_update',
      arguments: {
        entry_id: entryId,
        statement: 'Growth plan is 1799 per month effective August',
      },
    });
    const content = result.content as { type: string; text: string }[];
    const body = JSON.parse(content[0].text) as {
      proposal_id: string;
      kind: string;
    };
    expect(body.kind).toBe('contradiction');

    const proposal = await admin.query(
      `select origin, routed_to from drift_proposals where id = $1`,
      [body.proposal_id],
    );
    expect(proposal.rows[0]).toEqual({ origin: 'agent', routed_to: ownerId });
  });

  it('refuses unauthenticated mcp connections', async () => {
    const anon = new Client({ name: 'anon', version: '1.0.0' });
    await expect(
      anon.connect(
        new StreamableHTTPClientTransport(new URL(`${baseUrl}/mcp`)),
      ),
    ).rejects.toThrow();
  });
});
