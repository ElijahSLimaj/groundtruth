import { createHash, randomBytes } from 'node:crypto';

import {
  Body,
  Controller,
  Delete,
  ForbiddenException,
  Get,
  NotFoundException,
  Param,
  ParseUUIDPipe,
  Post,
  UseGuards,
} from '@nestjs/common';
import {
  IsArray,
  IsIn,
  IsNotEmpty,
  IsOptional,
  IsString,
} from 'class-validator';

import { InternalGuard } from '../auth/internal.guard';
import { CurrentPrincipal } from '../auth/principal';
import type { Principal } from '../auth/principal';
import { DatabaseService } from '../database/database.service';

class IssueKeyDto {
  @IsString()
  @IsNotEmpty()
  name!: string;

  @IsOptional()
  @IsIn(['standard', 'high', 'minimal'])
  rate_tier?: string;

  @IsOptional()
  @IsArray()
  @IsString({ each: true })
  allowed_domains?: string[];
}

@Controller('account')
@UseGuards(InternalGuard)
export class AccountController {
  constructor(private readonly db: DatabaseService) {}

  private requireManager(principal: Principal): void {
    if (principal.role !== 'admin' && principal.role !== 'owner') {
      throw new ForbiddenException(
        'only an admin or owner can manage the account',
      );
    }
  }

  @Get()
  async overview(@CurrentPrincipal() principal: Principal): Promise<{
    plan: string | null;
    subscription_status: string | null;
    included_query_volume: number | null;
    usage_this_month: number;
    keys: { id: string; name: string; rate_tier: string; created_at: string }[];
  }> {
    return this.db.withTenant(principal.tenantId, async (client) => {
      const tenant = await client.query<{
        plan: string | null;
        subscription_status: string | null;
        included_query_volume: number | null;
      }>(
        `select plan, subscription_status, included_query_volume
         from tenants where id = $1`,
        [principal.tenantId],
      );
      const usage = await client.query<{ n: string }>(
        `select count(*) as n from metering_events
         where billable and occurred_at >= date_trunc('month', now())`,
      );
      const keys = await client.query<{
        id: string;
        name: string;
        rate_tier: string;
        created_at: string;
      }>(
        `select id, name, rate_tier, created_at from api_keys
         where revoked_at is null order by created_at desc`,
      );
      return {
        plan: tenant.rows[0]?.plan ?? null,
        subscription_status: tenant.rows[0]?.subscription_status ?? null,
        included_query_volume: tenant.rows[0]?.included_query_volume ?? null,
        usage_this_month: Number(usage.rows[0].n),
        keys: keys.rows,
      };
    });
  }

  @Post('keys')
  async issueKey(
    @CurrentPrincipal() principal: Principal,
    @Body() dto: IssueKeyDto,
  ): Promise<{ id: string; key: string }> {
    this.requireManager(principal);
    const key = `cbk_${randomBytes(24).toString('hex')}`;
    const hash = createHash('sha256').update(key).digest('hex');
    const id = await this.db.withTenant(principal.tenantId, async (client) => {
      const inserted = await client.query<{ id: string }>(
        `insert into api_keys (tenant_id, person_id, key_hash, name, allowed_domains, rate_tier)
         values ($1, $2, $3, $4, $5, $6) returning id`,
        [
          principal.tenantId,
          principal.personId,
          hash,
          dto.name,
          dto.allowed_domains ?? null,
          dto.rate_tier ?? 'standard',
        ],
      );
      await client.query(
        `insert into audit_log (tenant_id, actor_id, action, subject_type, subject_id, detail)
         values ($1, $2, 'account.key.created', 'api_key', $3, $4)`,
        [
          principal.tenantId,
          principal.personId,
          inserted.rows[0].id,
          JSON.stringify({ name: dto.name }),
        ],
      );
      return inserted.rows[0].id;
    });
    return { id, key };
  }

  @Delete('keys/:id')
  async revokeKey(
    @CurrentPrincipal() principal: Principal,
    @Param('id', new ParseUUIDPipe({ errorHttpStatusCode: 404 })) id: string,
  ): Promise<{ revoked: boolean }> {
    this.requireManager(principal);
    await this.db.withTenant(principal.tenantId, async (client) => {
      const updated = await client.query(
        `update api_keys set revoked_at = now() where id = $1 and revoked_at is null`,
        [id],
      );
      if (updated.rowCount === 0) {
        throw new NotFoundException('key not found or already revoked');
      }
      await client.query(
        `insert into audit_log (tenant_id, actor_id, action, subject_type, subject_id)
         values ($1, $2, 'account.key.revoked', 'api_key', $3)`,
        [principal.tenantId, principal.personId, id],
      );
    });
    return { revoked: true };
  }
}
