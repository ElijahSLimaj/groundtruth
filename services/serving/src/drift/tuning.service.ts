import { Injectable, Logger } from '@nestjs/common';

import { DatabaseService } from '../database/database.service';
import { CALIBRATION_BINS, DEFAULT_TUNING } from './schemas';

const LOOKBACK_DAYS = 90;
const MIN_RESOLUTIONS_FOR_CURVE = 20;
const MIN_BIN_SAMPLES = 5;
const WRONG_PENALTY_STEP = 0.01;
const WRONG_PENALTY_CAP = 0.05;
const DUPLICATE_COOLDOWN_STEP = 2;
const DUPLICATE_COOLDOWN_CAP = 14;
const GAP_SIZE_STEP_DIVISOR = 3;
const GAP_SIZE_CAP = 5;

export interface TuningRunResult {
  resolutionsAnalyzed: number;
  domainsPenalized: number;
  cooldownDays: number;
  gapClusterMinSize: number;
  curveUpdated: boolean;
  badDraftFlags: number;
}

interface ResolutionRow {
  kind: string;
  resolution: string;
  domain: string | null;
  confidence: string;
}

@Injectable()
export class TuningService {
  private readonly logger = new Logger(TuningService.name);

  constructor(private readonly db: DatabaseService) {}

  async recalculate(tenantId: string): Promise<TuningRunResult> {
    return this.db.withTenant(tenantId, async (client) => {
      const rows = await client.query<ResolutionRow>(
        `select kind, resolution, domain, confidence from drift_proposals
         where status = 'resolved' and origin = 'drift_engine'
           and resolved_at > now() - make_interval(days => $1)`,
        [LOOKBACK_DAYS],
      );
      const resolutions = rows.rows;

      const wrongByDomain = new Map<string, number>();
      let duplicates = 0;
      let gapNotWorthy = 0;
      let badDrafts = 0;
      for (const row of resolutions) {
        if (row.resolution === 'wrong' && row.domain) {
          wrongByDomain.set(
            row.domain,
            (wrongByDomain.get(row.domain) ?? 0) + 1,
          );
        }
        if (row.resolution === 'duplicate') {
          duplicates++;
        }
        if (row.resolution === 'not_canon_worthy' && row.kind === 'gap') {
          gapNotWorthy++;
        }
        if (row.resolution === 'bad_draft') {
          badDrafts++;
        }
      }

      const penalties: Record<string, number> = {};
      for (const [domain, count] of wrongByDomain) {
        penalties[domain] = Math.min(
          WRONG_PENALTY_CAP,
          count * WRONG_PENALTY_STEP,
        );
      }
      const cooldownDays =
        DEFAULT_TUNING.cooldown_days +
        Math.min(DUPLICATE_COOLDOWN_CAP, duplicates * DUPLICATE_COOLDOWN_STEP);
      const gapClusterMinSize =
        DEFAULT_TUNING.gap_cluster_min_size +
        Math.min(
          GAP_SIZE_CAP,
          Math.floor(gapNotWorthy / GAP_SIZE_STEP_DIVISOR),
        );

      const curve = this.buildCurve(
        resolutions.filter(
          (r) => r.kind === 'contradiction' || r.kind === 'extension',
        ),
      );

      const computed: Record<string, unknown> = {
        tier1_domain_penalties: penalties,
        cooldown_days: cooldownDays,
        gap_cluster_min_size: gapClusterMinSize,
        tier3_calibration: curve,
      };
      await client.query(
        `insert into drift_tuning (tenant_id, params)
         values ($1, $2)
         on conflict (tenant_id) do update
         set params = drift_tuning.params || excluded.params, updated_at = now()`,
        [tenantId, JSON.stringify(computed)],
      );

      if (badDrafts > 0) {
        this.logger.warn(
          JSON.stringify({
            event: 'tuning_prompt_regression_flagged',
            tenant: tenantId,
            bad_draft_rejections: badDrafts,
          }),
        );
      }
      await client.query(
        `insert into audit_log (tenant_id, actor_id, action, subject_type, subject_id, detail)
         values ($1, null, 'tuning.recalculated', 'drift_tuning', $1, $2)`,
        [
          tenantId,
          JSON.stringify({
            resolutions_analyzed: resolutions.length,
            ...computed,
            bad_draft_rejections: badDrafts,
          }),
        ],
      );

      return {
        resolutionsAnalyzed: resolutions.length,
        domainsPenalized: wrongByDomain.size,
        cooldownDays,
        gapClusterMinSize,
        curveUpdated: curve !== null,
        badDraftFlags: badDrafts,
      };
    });
  }

  private buildCurve(resolutions: ResolutionRow[]): (number | null)[] | null {
    if (resolutions.length < MIN_RESOLUTIONS_FOR_CURVE) {
      return null;
    }
    const totals = new Array<number>(CALIBRATION_BINS).fill(0);
    const approved = new Array<number>(CALIBRATION_BINS).fill(0);
    for (const row of resolutions) {
      const confidence = Number(row.confidence);
      const bin = Math.min(
        CALIBRATION_BINS - 1,
        Math.floor(confidence * CALIBRATION_BINS),
      );
      totals[bin]++;
      if (row.resolution === 'approved') {
        approved[bin]++;
      }
    }
    return totals.map((total, bin) =>
      total >= MIN_BIN_SAMPLES ? approved[bin] / total : null,
    );
  }
}
