import { unlink } from 'node:fs/promises';
import { join } from 'node:path';

import {
  BadRequestException,
  Inject,
  Injectable,
  Logger,
  NotFoundException,
} from '@nestjs/common';

import { SERVING_CONFIG } from '../config';
import type { ServingConfig } from '../config';
import { DatabaseService } from '../database/database.service';

const PAYLOAD_REF_PATTERN =
  /^payloads\/([0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12})\/([0-9a-f]{64})$/;

export interface ErasureRequestRow {
  id: string;
  person_id: string;
  status: string;
  reason: string;
  created_at: string;
}

@Injectable()
export class ErasureService {
  private readonly logger = new Logger(ErasureService.name);

  constructor(
    private readonly db: DatabaseService,
    @Inject(SERVING_CONFIG) private readonly config: ServingConfig,
  ) {}

  async create(
    tenantId: string,
    requesterId: string,
    personId: string,
    reason: string,
  ): Promise<ErasureRequestRow> {
    return this.db.withTenant(tenantId, async (client) => {
      const person = await client.query(`select 1 from people where id = $1`, [
        personId,
      ]);
      if (person.rowCount === 0) {
        throw new NotFoundException('person not found');
      }
      const inserted = await client.query<ErasureRequestRow>(
        `insert into erasure_requests (tenant_id, person_id, requested_by, reason)
         values ($1, $2, $3, $4)
         returning id, person_id, status, reason, created_at`,
        [tenantId, personId, requesterId, reason],
      );
      await client.query(
        `insert into audit_log (tenant_id, actor_id, action, subject_type, subject_id, detail)
         values ($1, $2, 'erasure.requested', 'erasure_request', $3, $4)`,
        [
          tenantId,
          requesterId,
          inserted.rows[0].id,
          JSON.stringify({ person_id: personId, reason }),
        ],
      );
      return inserted.rows[0];
    });
  }

  async review(
    tenantId: string,
    reviewerId: string,
    requestId: string,
    decision: 'verified' | 'rejected',
  ): Promise<void> {
    await this.db.withTenant(tenantId, async (client) => {
      const updated = await client.query(
        `update erasure_requests
         set status = $2, verified_by = $3, verified_at = now()
         where id = $1 and status = 'pending'`,
        [requestId, decision, reviewerId],
      );
      if (updated.rowCount === 0) {
        throw new NotFoundException('erasure request not found or not pending');
      }
      await client.query(
        `insert into audit_log (tenant_id, actor_id, action, subject_type, subject_id, detail)
         values ($1, $2, $3, 'erasure_request', $4, '{}')`,
        [tenantId, reviewerId, `erasure.${decision}`, requestId],
      );
    });
  }

  async execute(
    tenantId: string,
    executorId: string,
    requestId: string,
  ): Promise<{ payloads_deleted: number }> {
    const refs = await this.db.withTenant(tenantId, async (client) => {
      const result = await client.query<{ payload_ref: string }>(
        `select * from public.erasure_execute($1, $2)`,
        [requestId, executorId],
      );
      return result.rows.map((r) => r.payload_ref);
    });
    const deleted = await this.deletePayloads(tenantId, refs);
    return { payloads_deleted: deleted };
  }

  async tombstoneEvent(
    tenantId: string,
    actorId: string,
    eventId: string,
  ): Promise<{ chunks_tombstoned: number }> {
    return this.db.withTenant(tenantId, async (client) => {
      const result = await client.query<{ event_tombstone: number }>(
        `select public.event_tombstone($1, $2)`,
        [eventId, actorId],
      );
      return { chunks_tombstoned: result.rows[0].event_tombstone };
    });
  }

  private async deletePayloads(
    tenantId: string,
    refs: string[],
  ): Promise<number> {
    if (refs.length === 0) {
      return 0;
    }
    if (!this.config.payloadRoot) {
      this.logger.warn(
        JSON.stringify({
          event: 'erasure_payloads_retained',
          tenant: tenantId,
          refs: refs.length,
          reason: 'PAYLOAD_ROOT not configured',
        }),
      );
      return 0;
    }
    let deleted = 0;
    for (const ref of refs) {
      const match = PAYLOAD_REF_PATTERN.exec(ref);
      if (!match) {
        throw new BadRequestException(`malformed payload ref ${ref}`);
      }
      if (match[1] !== tenantId) {
        throw new BadRequestException(
          `payload ref ${ref} does not belong to tenant ${tenantId}`,
        );
      }
      try {
        await unlink(join(this.config.payloadRoot, match[1], match[2]));
        deleted += 1;
      } catch (error) {
        if ((error as NodeJS.ErrnoException).code !== 'ENOENT') {
          throw error;
        }
      }
    }
    return deleted;
  }
}
