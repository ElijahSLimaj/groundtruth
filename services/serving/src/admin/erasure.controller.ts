import {
  Body,
  Controller,
  ForbiddenException,
  Param,
  ParseUUIDPipe,
  Post,
  UseGuards,
} from '@nestjs/common';
import { IsNotEmpty, IsString, IsUUID } from 'class-validator';

import { ApiKeyGuard } from '../auth/api-key.guard';
import { CurrentPrincipal } from '../auth/principal';
import type { Principal } from '../auth/principal';
import { ErasureService } from './erasure.service';
import type { ErasureRequestRow } from './erasure.service';

class CreateErasureDto {
  @IsUUID()
  person_id!: string;

  @IsString()
  @IsNotEmpty()
  reason!: string;
}

function requireAdmin(principal: Principal): void {
  if (principal.role !== 'admin') {
    throw new ForbiddenException('admin role required');
  }
}

@Controller('admin/erasure')
@UseGuards(ApiKeyGuard)
export class ErasureController {
  constructor(private readonly erasure: ErasureService) {}

  @Post()
  async create(
    @CurrentPrincipal() principal: Principal,
    @Body() dto: CreateErasureDto,
  ): Promise<ErasureRequestRow> {
    requireAdmin(principal);
    return this.erasure.create(
      principal.tenantId,
      principal.personId,
      dto.person_id,
      dto.reason,
    );
  }

  @Post(':id/verify')
  async verify(
    @CurrentPrincipal() principal: Principal,
    @Param('id', new ParseUUIDPipe({ errorHttpStatusCode: 404 })) id: string,
  ): Promise<{ status: string }> {
    requireAdmin(principal);
    await this.erasure.review(
      principal.tenantId,
      principal.personId,
      id,
      'verified',
    );
    return { status: 'verified' };
  }

  @Post(':id/reject')
  async reject(
    @CurrentPrincipal() principal: Principal,
    @Param('id', new ParseUUIDPipe({ errorHttpStatusCode: 404 })) id: string,
  ): Promise<{ status: string }> {
    requireAdmin(principal);
    await this.erasure.review(
      principal.tenantId,
      principal.personId,
      id,
      'rejected',
    );
    return { status: 'rejected' };
  }

  @Post(':id/execute')
  async execute(
    @CurrentPrincipal() principal: Principal,
    @Param('id', new ParseUUIDPipe({ errorHttpStatusCode: 404 })) id: string,
  ): Promise<{ status: string; payloads_deleted: number }> {
    requireAdmin(principal);
    const result = await this.erasure.execute(
      principal.tenantId,
      principal.personId,
      id,
    );
    return { status: 'completed', ...result };
  }
}

@Controller('admin/events')
@UseGuards(ApiKeyGuard)
export class TombstoneController {
  constructor(private readonly erasure: ErasureService) {}

  @Post(':id/tombstone')
  async tombstone(
    @CurrentPrincipal() principal: Principal,
    @Param('id', new ParseUUIDPipe({ errorHttpStatusCode: 404 })) id: string,
  ): Promise<{ tombstoned: boolean; chunks_tombstoned: number }> {
    requireAdmin(principal);
    const result = await this.erasure.tombstoneEvent(
      principal.tenantId,
      principal.personId,
      id,
    );
    return { tombstoned: true, ...result };
  }
}
