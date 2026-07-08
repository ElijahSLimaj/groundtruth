import {
  Inject,
  Injectable,
  Logger,
  OnModuleDestroy,
  OnModuleInit,
} from '@nestjs/common';

import { ColdStartService } from '../coldstart/coldstart.service';
import { DatabaseService } from '../database/database.service';
import { DriftService } from '../drift/drift.service';
import { GapService } from '../drift/gap.service';
import { MergeService } from '../drift/merge.service';
import { SERVING_CONFIG } from '../config';
import type { ServingConfig } from '../config';
import { SlackAppService } from '../slackapp/slackapp.service';

@Injectable()
export class SchedulerService implements OnModuleInit, OnModuleDestroy {
  private readonly logger = new Logger(SchedulerService.name);
  private timers: NodeJS.Timeout[] = [];
  private running = false;

  constructor(
    private readonly db: DatabaseService,
    private readonly drift: DriftService,
    private readonly gap: GapService,
    private readonly merge: MergeService,
    private readonly coldStart: ColdStartService,
    private readonly slackApp: SlackAppService,
    @Inject(SERVING_CONFIG) private readonly config: ServingConfig,
  ) {}

  onModuleInit(): void {
    if (!this.config.schedulerEnabled) {
      this.logger.log('scheduler disabled, set SCHEDULER_ENABLED=1 to run');
      return;
    }
    this.timers.push(
      setInterval(() => {
        void this.sweep();
      }, this.config.driftIntervalMs),
    );
    this.timers.push(
      setInterval(() => {
        void this.decaySweep();
      }, this.config.decayIntervalMs),
    );
    this.timers.push(
      setInterval(() => {
        void this.mergeSweep();
      }, this.config.mergeIntervalMs),
    );
    this.logger.log(
      JSON.stringify({
        event: 'scheduler_started',
        drift_interval_ms: this.config.driftIntervalMs,
        decay_interval_ms: this.config.decayIntervalMs,
        merge_interval_ms: this.config.mergeIntervalMs,
      }),
    );
  }

  onModuleDestroy(): void {
    for (const timer of this.timers) {
      clearInterval(timer);
    }
  }

  async tenants(): Promise<string[]> {
    return this.db.asApp(async (client) => {
      const rows = await client.query<{ list_tenant_ids: string }>(
        `select * from public.list_tenant_ids()`,
      );
      return rows.rows.map((r) => r.list_tenant_ids);
    });
  }

  async sweep(): Promise<void> {
    if (this.running) {
      return;
    }
    this.running = true;
    try {
      for (const tenantId of await this.tenants()) {
        try {
          const drift = await this.drift.runOnce(tenantId);
          const gaps = await this.gap.runOnce(tenantId);
          const coldStart = await this.coldStart.runOnce(tenantId);
          const notified = await this.slackApp.notifyPending(tenantId);
          this.logger.log(
            JSON.stringify({
              event: 'sweep',
              tenant: tenantId,
              drift,
              gaps,
              cold_start: coldStart,
              slack_notified: notified,
            }),
          );
        } catch (error) {
          this.logger.error(
            JSON.stringify({
              event: 'sweep_failed',
              tenant: tenantId,
              error: String(error),
            }),
          );
        }
      }
    } finally {
      this.running = false;
    }
  }

  async mergeSweep(): Promise<void> {
    for (const tenantId of await this.tenants()) {
      try {
        const proposed = await this.merge.runOnce(tenantId);
        if (proposed > 0) {
          this.logger.log(
            JSON.stringify({
              event: 'merge_sweep',
              tenant: tenantId,
              proposed,
            }),
          );
        }
      } catch (error) {
        this.logger.error(
          JSON.stringify({
            event: 'merge_sweep_failed',
            tenant: tenantId,
            error: String(error),
          }),
        );
      }
    }
  }

  async decaySweep(): Promise<void> {
    for (const tenantId of await this.tenants()) {
      try {
        const decayed = await this.db.withTenant(tenantId, async (client) => {
          const result = await client.query<{ canon_decay_sweep: number }>(
            `select public.canon_decay_sweep()`,
          );
          return result.rows[0].canon_decay_sweep;
        });
        if (decayed > 0) {
          this.logger.log(
            JSON.stringify({ event: 'decay_sweep', tenant: tenantId, decayed }),
          );
        }
      } catch (error) {
        this.logger.error(
          JSON.stringify({
            event: 'decay_sweep_failed',
            tenant: tenantId,
            error: String(error),
          }),
        );
      }
    }
  }
}
