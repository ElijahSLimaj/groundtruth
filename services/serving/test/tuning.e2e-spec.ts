import { randomUUID } from 'node:crypto';

import { Test } from '@nestjs/testing';
import { Pool } from 'pg';

import { AppModule } from '../src/app.module';
import { TuningService } from '../src/drift/tuning.service';

const databaseUrl = process.env.TEST_DATABASE_URL;
const suite = databaseUrl ? describe : describe.skip;

suite('weekly tuning recalculation (e2e)', () => {
  let admin: Pool;
  let tuning: TuningService;
  let closeApp: () => Promise<void>;

  const tenantId = randomUUID();
  const ownerId = randomUUID();

  const seedResolved = async (
    kind: string,
    resolution: string,
    confidence: number,
    count: number,
    domain = 'pricing',
  ) => {
    for (let i = 0; i < count; i++) {
      await admin.query(
        `insert into drift_proposals
           (tenant_id, kind, drafted_statement, confidence, routed_to, domain,
            origin, status, resolution, resolved_at)
         values ($1, $2, 'seeded statement', $3, $4, $5, 'drift_engine', 'resolved', $6, now())`,
        [tenantId, kind, confidence, ownerId, domain, resolution],
      );
    }
  };

  beforeAll(async () => {
    admin = new Pool({ connectionString: databaseUrl });
    await admin.query(
      `insert into tenants (id, name, tier) values ($1, 'Tuning E2E', 'growth')`,
      [tenantId],
    );
    await admin.query(
      `insert into people (id, tenant_id, email, display_name, role)
       values ($1, $2, 'owner@tuning.test', 'Owner', 'owner')`,
      [ownerId, tenantId],
    );
    await admin.query(
      `insert into drift_tuning (tenant_id, params) values ($1, '{"tier2_gate": 0.9}')`,
      [tenantId],
    );

    await seedResolved('contradiction', 'approved', 0.9, 10);
    await seedResolved('contradiction', 'wrong', 0.9, 5);
    await seedResolved('contradiction', 'wrong', 0.1, 6);
    await seedResolved('contradiction', 'duplicate', 0.5, 2);
    await seedResolved('contradiction', 'bad_draft', 0.5, 3);
    await seedResolved('gap', 'not_canon_worthy', 0.6, 7);

    process.env.DATABASE_URL = databaseUrl;
    const moduleRef = await Test.createTestingModule({
      imports: [AppModule],
    }).compile();
    const app = moduleRef.createNestApplication();
    await app.init();
    tuning = app.get(TuningService);
    closeApp = () => app.close();
  });

  afterAll(async () => {
    if (closeApp) {
      await closeApp();
    }
    for (const stmt of [
      `delete from drift_proposals where tenant_id = $1`,
      `delete from drift_tuning where tenant_id = $1`,
      `delete from audit_log where tenant_id = $1`,
      `delete from people where tenant_id = $1`,
      `delete from tenants where id = $1`,
    ]) {
      await admin.query(stmt, [tenantId]);
    }
    await admin.end();
  });

  it('recalculates thresholds and the calibration curve from the rejection taxonomy', async () => {
    const result = await tuning.recalculate(tenantId);

    expect(result.resolutionsAnalyzed).toBe(33);
    expect(result.domainsPenalized).toBe(1);
    expect(result.cooldownDays).toBe(18);
    expect(result.gapClusterMinSize).toBe(7);
    expect(result.curveUpdated).toBe(true);
    expect(result.badDraftFlags).toBe(3);

    const stored = await admin.query<{
      params: {
        tier1_domain_penalties: Record<string, number>;
        cooldown_days: number;
        gap_cluster_min_size: number;
        tier3_calibration: (number | null)[];
        tier2_gate: number;
      };
    }>(`select params from drift_tuning where tenant_id = $1`, [tenantId]);
    const params = stored.rows[0].params;

    expect(params.tier1_domain_penalties.pricing).toBe(0.05);
    expect(params.cooldown_days).toBe(18);
    expect(params.gap_cluster_min_size).toBe(7);
    expect(params.tier2_gate).toBe(0.9);

    const curve = params.tier3_calibration;
    expect(curve[0]).toBe(0);
    expect(curve[1]).toBeNull();
    expect(curve[2]).toBe(0);
    expect(curve[3]).toBeNull();
    expect(curve[4]).toBeCloseTo(10 / 15, 5);

    const audit = await admin.query(
      `select count(*) as n from audit_log
       where tenant_id = $1 and action = 'tuning.recalculated'`,
      [tenantId],
    );
    expect(Number(audit.rows[0].n)).toBe(1);
  });

  it('keeps the global identity curve when history is thin', async () => {
    await admin.query(
      `delete from drift_proposals where tenant_id = $1 and kind = 'contradiction' and resolution = 'approved'`,
      [tenantId],
    );
    await admin.query(
      `delete from drift_proposals where tenant_id = $1 and resolution = 'wrong'`,
      [tenantId],
    );

    const result = await tuning.recalculate(tenantId);
    expect(result.curveUpdated).toBe(false);

    const stored = await admin.query<{
      params: { tier3_calibration: (number | null)[] | null };
    }>(`select params from drift_tuning where tenant_id = $1`, [tenantId]);
    expect(stored.rows[0].params.tier3_calibration).toBeNull();
  });
});
