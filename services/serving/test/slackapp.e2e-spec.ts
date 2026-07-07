import { createHmac, randomUUID } from 'node:crypto';

import { INestApplication } from '@nestjs/common';
import { Test } from '@nestjs/testing';
import { Pool } from 'pg';
import request from 'supertest';

import { AppModule } from '../src/app.module';
import { configureApp } from '../src/configure-app';
import { SlackAppService } from '../src/slackapp/slackapp.service';
import { SLACK_WEB_API } from '../src/slackapp/slack-web';
import type { SlackMessage, SlackWebApi } from '../src/slackapp/slack-web';

const databaseUrl = process.env.TEST_DATABASE_URL;
const suite = databaseUrl ? describe : describe.skip;

class FakeSlackWebApi implements SlackWebApi {
  messages: SlackMessage[] = [];

  postMessage(message: SlackMessage): Promise<void> {
    this.messages.push(message);
    return Promise.resolve();
  }
}

const SIGNING_SECRET = 'test-signing-secret';

function signedBody(payload: unknown): {
  body: string;
  headers: Record<string, string>;
} {
  const body = 'payload=' + encodeURIComponent(JSON.stringify(payload));
  const timestamp = String(Math.floor(Date.now() / 1000));
  const signature =
    'v0=' +
    createHmac('sha256', SIGNING_SECRET)
      .update(`v0:${timestamp}:${body}`)
      .digest('hex');
  return {
    body,
    headers: {
      'x-slack-signature': signature,
      'x-slack-request-timestamp': timestamp,
      'content-type': 'application/x-www-form-urlencoded',
    },
  };
}

suite('slack approval app (e2e)', () => {
  let app: INestApplication;
  let admin: Pool;
  let slackWeb: FakeSlackWebApi;
  let slackApp: SlackAppService;
  let httpServer: Parameters<typeof request>[0];

  const tenantId = randomUUID();
  const ownerId = randomUUID();
  const entryId = randomUUID();
  const versionId = randomUUID();
  const proposalId = randomUUID();

  beforeAll(async () => {
    admin = new Pool({ connectionString: databaseUrl });
    await admin.query(
      `insert into tenants (id, name, tier) values ($1, 'Slack E2E', 'growth')`,
      [tenantId],
    );
    await admin.query(
      `insert into people (id, tenant_id, email, display_name, role, slack_user_id)
       values ($1, $2, 'owner@slack.test', 'Owner', 'owner', 'U0OWNER')`,
      [ownerId, tenantId],
    );
    await admin.query(
      `insert into canon_entries (id, tenant_id, domain, tier, owner_id, status, visibility, verify_interval, verified_at)
       values ($1, $2, 'pricing', 'operational', $3, 'active', '{"scope": "tenant"}', interval '60 days', now())`,
      [entryId, tenantId, ownerId],
    );
    await admin.query(
      `insert into canon_versions (id, tenant_id, entry_id, version_number, statement, created_by, status)
       values ($1, $2, $3, 1, 'Growth is 1499 per month', $4, 'approved')`,
      [versionId, tenantId, entryId, ownerId],
    );
    await admin.query(
      `update canon_entries set current_version_id = $2 where id = $1`,
      [entryId, versionId],
    );
    await admin.query(
      `insert into drift_proposals (id, tenant_id, entry_id, kind, drafted_statement, confidence, routed_to, domain)
       values ($1, $2, $3, 'contradiction', 'Growth is 1799 per month from August', 0.9, $4, 'pricing')`,
      [proposalId, tenantId, entryId, ownerId],
    );

    process.env.DATABASE_URL = databaseUrl;
    process.env.SLACK_BOT_TOKEN = 'xoxb-test';
    process.env.SLACK_SIGNING_SECRET = SIGNING_SECRET;
    process.env.SLACK_APPROVAL_CHANNEL = 'C0QUEUE';
    process.env.SLACK_TENANT_ID = tenantId;

    slackWeb = new FakeSlackWebApi();
    const moduleRef = await Test.createTestingModule({ imports: [AppModule] })
      .overrideProvider(SLACK_WEB_API)
      .useValue(slackWeb)
      .compile();
    app = configureApp(moduleRef.createNestApplication({ rawBody: true }));
    await app.init();
    slackApp = app.get(SlackAppService);
    httpServer = app.getHttpServer() as Parameters<typeof request>[0];
  });

  afterAll(async () => {
    delete process.env.SLACK_BOT_TOKEN;
    delete process.env.SLACK_SIGNING_SECRET;
    delete process.env.SLACK_APPROVAL_CHANNEL;
    delete process.env.SLACK_TENANT_ID;
    if (app) {
      await app.close();
    }
    for (const stmt of [
      `delete from drift_evidence where tenant_id = $1`,
      `delete from drift_proposals where tenant_id = $1`,
      `delete from audit_log where tenant_id = $1`,
      `delete from approvals where tenant_id = $1`,
      `delete from canon_provenance where tenant_id = $1`,
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

  it('notifies pending proposals once with approve and reject actions', async () => {
    const sent = await slackApp.notifyPending(tenantId);
    expect(sent).toBe(1);
    expect(slackWeb.messages).toHaveLength(1);
    const blocks = JSON.stringify(slackWeb.messages[0].blocks);
    expect(blocks).toContain('1799');
    expect(blocks).toContain('approve_proposal');
    expect(blocks).toContain('reject_proposal');

    const again = await slackApp.notifyPending(tenantId);
    expect(again).toBe(0);
  });

  it('rejects interactions with a bad signature', async () => {
    const { body } = signedBody({ type: 'block_actions' });
    await request(httpServer)
      .post('/slack/interactions')
      .set({
        'x-slack-signature': 'v0=deadbeef',
        'x-slack-request-timestamp': String(Math.floor(Date.now() / 1000)),
        'content-type': 'application/x-www-form-urlencoded',
      })
      .send(body)
      .expect(401);
  });

  it('tells unlinked slack users how to get access', async () => {
    const { body, headers } = signedBody({
      type: 'block_actions',
      user: { id: 'U_STRANGER' },
      actions: [{ action_id: 'approve_proposal', value: proposalId }],
    });
    const res = await request(httpServer)
      .post('/slack/interactions')
      .set(headers)
      .send(body)
      .expect(200);
    expect(res.body.text).toContain('not linked');
  });

  it('approves through the same canon transaction as the web queue', async () => {
    const { body, headers } = signedBody({
      type: 'block_actions',
      user: { id: 'U0OWNER' },
      actions: [{ action_id: 'approve_proposal', value: proposalId }],
    });
    const res = await request(httpServer)
      .post('/slack/interactions')
      .set(headers)
      .send(body)
      .expect(200);
    expect(res.body.text).toContain('Approved by Owner');

    const proposal = await admin.query(
      `select status, resolution from drift_proposals where id = $1`,
      [proposalId],
    );
    expect(proposal.rows[0]).toEqual({
      status: 'resolved',
      resolution: 'approved',
    });

    const entry = await admin.query(
      `select cv.statement, cv.version_number
       from canon_entries ce join canon_versions cv on cv.id = ce.current_version_id
       where ce.id = $1`,
      [entryId],
    );
    expect(entry.rows[0].statement).toContain('1799');
    expect(entry.rows[0].version_number).toBe(2);
  });
});
