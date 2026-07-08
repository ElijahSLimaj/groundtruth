import { Inject, Injectable } from '@nestjs/common';

import { SERVING_CONFIG } from '../config';
import type { ServingConfig } from '../config';
import { DatabaseService } from '../database/database.service';

@Injectable()
export class MergeService {
  constructor(
    private readonly db: DatabaseService,
    @Inject(SERVING_CONFIG) private readonly config: ServingConfig,
  ) {}

  async runOnce(tenantId: string): Promise<number> {
    return this.db.withTenant(tenantId, async (client) => {
      const result = await client.query<{ canon_merge_detect: number }>(
        `select public.canon_merge_detect($1, $2)`,
        [this.config.embeddingModel, this.config.mergeSimilarityThreshold],
      );
      return result.rows[0].canon_merge_detect;
    });
  }
}
