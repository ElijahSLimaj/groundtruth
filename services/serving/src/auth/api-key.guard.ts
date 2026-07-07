import { createHash } from 'node:crypto';

import {
  CanActivate,
  ExecutionContext,
  HttpException,
  Injectable,
  UnauthorizedException,
} from '@nestjs/common';
import { Response } from 'express';

import { DatabaseService } from '../database/database.service';
import { AuthenticatedRequest } from './principal';
import { RateLimiterService } from './rate-limiter.service';

interface KeyRow {
  key_id: string;
  tenant_id: string;
  person_id: string;
  person_role: string;
  display_name: string;
  allowed_domains: string[] | null;
  rate_tier: string;
}

@Injectable()
export class ApiKeyGuard implements CanActivate {
  constructor(
    private readonly db: DatabaseService,
    private readonly limiter: RateLimiterService,
  ) {}

  async canActivate(context: ExecutionContext): Promise<boolean> {
    const request = context.switchToHttp().getRequest<AuthenticatedRequest>();
    const header = request.headers.authorization;
    if (typeof header !== 'string' || !header.startsWith('Bearer ')) {
      throw new UnauthorizedException('missing api key');
    }
    const hash = createHash('sha256')
      .update(header.slice('Bearer '.length))
      .digest('hex');

    const result = await this.db.asApp((client) =>
      client.query<KeyRow>('select * from public.api_key_lookup($1)', [hash]),
    );
    const row = result.rows[0];
    if (!row) {
      throw new UnauthorizedException('unknown or revoked api key');
    }

    const verdict = this.limiter.take(row.key_id, row.rate_tier);
    if (!verdict.allowed) {
      const response = context.switchToHttp().getResponse<Response>();
      response.setHeader('Retry-After', String(verdict.retryAfterSeconds));
      throw new HttpException(
        {
          statusCode: 429,
          message: 'rate limited',
          retryAfter: verdict.retryAfterSeconds,
        },
        429,
      );
    }

    request.principal = {
      keyId: row.key_id,
      tenantId: row.tenant_id,
      personId: row.person_id,
      role: row.person_role,
      displayName: row.display_name,
      allowedDomains: row.allowed_domains,
      rateTier: row.rate_tier,
      principals: [`person:${row.person_id}`],
    };
    return true;
  }
}
