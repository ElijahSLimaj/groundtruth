import { BadRequestException, Inject, Injectable } from '@nestjs/common';

import { SERVING_CONFIG } from '../config';
import type { ServingConfig } from '../config';
import { DatabaseService } from '../database/database.service';
import { ConnectResult, writeConnectorSecret } from './connector-writer';
import { providerFor } from './providers';

@Injectable()
export class ApiKeyService {
  constructor(
    private readonly db: DatabaseService,
    @Inject(SERVING_CONFIG) private readonly config: ServingConfig,
  ) {}

  async store(
    tenantId: string,
    source: string,
    apiKey: string,
    baseUrl: string | null,
  ): Promise<ConnectResult> {
    const provider = providerFor(source);
    if (!provider || provider.kind !== 'apikey') {
      throw new BadRequestException(`${source} is not an API-key connector`);
    }
    if (!apiKey) {
      throw new BadRequestException('api_key is required');
    }
    if (provider.needsBaseUrl && !baseUrl) {
      throw new BadRequestException(`${source} requires base_url`);
    }
    if (!this.config.masterKeyHex) {
      throw new BadRequestException('MASTER_KEY is not configured');
    }

    return writeConnectorSecret({
      db: this.db,
      masterKeyHex: this.config.masterKeyHex,
      tenantId,
      source,
      secret: { access_token: apiKey, refresh_token: null },
      clearConfig: { expires_at: null, source_scope: { base_url: baseUrl } },
    });
  }
}
