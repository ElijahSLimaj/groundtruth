import { createHash, randomBytes } from 'node:crypto';

import {
  Body,
  Controller,
  Delete,
  ForbiddenException,
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
  IsUUID,
} from 'class-validator';

import { ApiKeyGuard } from '../auth/api-key.guard';
import { CurrentPrincipal } from '../auth/principal';
import type { Principal } from '../auth/principal';
import { DatabaseService } from '../database/database.service';

class CreateKeyDto {
  @IsUUID()
  person_id!: string;

  @IsString()
  @IsNotEmpty()
  name!: string;

  @IsOptional()
  @IsArray()
  @IsString({ each: true })
  allowed_domains?: string[];

  @IsOptional()
  @IsIn(['standard', 'high', 'minimal'])
  rate_tier?: string;
}

@Controller('admin/keys')
@UseGuards(ApiKeyGuard)
export class AdminController {
  constructor(private readonly db: DatabaseService) {}

  @Post()
  async createKey(
    @CurrentPrincipal() principal: Principal,
    @Body() dto: CreateKeyDto,
  ): Promise<{ id: string; key: string }> {
    this.requireAdmin(principal);
    const key = `cbk_${randomBytes(24).toString('hex')}`;
    const hash = createHash('sha256').update(key).digest('hex');

    const id = await this.db.withTenant(principal.tenantId, async (client) => {
      const person = await client.query(`select 1 from people where id = $1`, [
        dto.person_id,
      ]);
      if (person.rowCount === 0) {
        throw new NotFoundException('person not found');
      }
      const inserted = await client.query<{ id: string }>(
        `insert into api_keys (tenant_id, person_id, key_hash, name, allowed_domains, rate_tier)
         values ($1, $2, $3, $4, $5, $6)
         returning id`,
        [
          principal.tenantId,
          dto.person_id,
          hash,
          dto.name,
          dto.allowed_domains ?? null,
          dto.rate_tier ?? 'standard',
        ],
      );
      await this.audit(
        client,
        principal,
        'admin.key.created',
        inserted.rows[0].id,
        {
          person_id: dto.person_id,
          name: dto.name,
        },
      );
      return inserted.rows[0].id;
    });
    return { id, key };
  }

  @Delete(':id')
  async revokeKey(
    @CurrentPrincipal() principal: Principal,
    @Param('id', new ParseUUIDPipe({ errorHttpStatusCode: 404 })) id: string,
  ): Promise<{ revoked: boolean }> {
    this.requireAdmin(principal);
    await this.db.withTenant(principal.tenantId, async (client) => {
      const updated = await client.query(
        `update api_keys set revoked_at = now() where id = $1 and revoked_at is null`,
        [id],
      );
      if (updated.rowCount === 0) {
        throw new NotFoundException('key not found or already revoked');
      }
      await this.audit(client, principal, 'admin.key.revoked', id, {});
    });
    return { revoked: true };
  }

  private requireAdmin(principal: Principal): void {
    if (principal.role !== 'admin') {
      throw new ForbiddenException('admin role required');
    }
  }

  private async audit(
    client: Parameters<Parameters<DatabaseService['withTenant']>[1]>[0],
    principal: Principal,
    action: string,
    subjectId: string,
    detail: Record<string, unknown>,
  ): Promise<void> {
    await client.query(
      `insert into audit_log (tenant_id, actor_id, action, subject_type, subject_id, detail)
       values ($1, $2, $3, 'api_key', $4, $5)`,
      [
        principal.tenantId,
        principal.personId,
        action,
        subjectId,
        JSON.stringify(detail),
      ],
    );
  }
}
