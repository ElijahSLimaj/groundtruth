import { timingSafeEqual } from 'node:crypto';

import {
  CanActivate,
  ExecutionContext,
  Inject,
  Injectable,
  UnauthorizedException,
} from '@nestjs/common';

import { SERVING_CONFIG } from '../config';
import type { ServingConfig } from '../config';
import { DatabaseService } from '../database/database.service';
import type { AuthenticatedRequest } from './principal';

const UUID_PATTERN =
  /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/;

@Injectable()
export class InternalGuard implements CanActivate {
  constructor(
    private readonly db: DatabaseService,
    @Inject(SERVING_CONFIG) private readonly config: ServingConfig,
  ) {}

  async canActivate(context: ExecutionContext): Promise<boolean> {
    const request = context.switchToHttp().getRequest<AuthenticatedRequest>();
    const secret = this.config.internalApiSecret;
    const provided = String(request.headers['x-internal-secret'] ?? '');
    if (!secret || provided.length !== secret.length) {
      throw new UnauthorizedException('internal access denied');
    }
    if (!timingSafeEqual(Buffer.from(provided), Buffer.from(secret))) {
      throw new UnauthorizedException('internal access denied');
    }

    const tenantId = String(request.headers['x-tenant-id'] ?? '');
    const personId = String(request.headers['x-person-id'] ?? '');
    if (!UUID_PATTERN.test(tenantId) || !UUID_PATTERN.test(personId)) {
      throw new UnauthorizedException('internal principal headers missing');
    }

    const person = await this.db.withTenant(tenantId, async (client) => {
      const rows = await client.query<{
        role: string;
        display_name: string;
      }>(`select role, display_name from people where id = $1`, [personId]);
      return rows.rows[0];
    });
    if (!person) {
      throw new UnauthorizedException('internal principal unknown');
    }

    request.principal = {
      keyId: 'internal',
      tenantId,
      personId,
      role: person.role,
      displayName: person.display_name,
      allowedDomains: null,
      rateTier: 'standard',
      principals: [`person:${personId}`],
    };
    return true;
  }
}
