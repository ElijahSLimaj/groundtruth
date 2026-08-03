import {
  Body,
  Controller,
  Delete,
  ForbiddenException,
  Get,
  Param,
  ParseUUIDPipe,
  Post,
  UseGuards,
} from '@nestjs/common';
import { IsOptional, IsString, IsNotEmpty } from 'class-validator';

import { InternalGuard } from '../auth/internal.guard';
import { CurrentPrincipal } from '../auth/principal';
import type { Principal } from '../auth/principal';
import { DatabaseService } from '../database/database.service';
import { ApiKeyService } from './apikey.service';
import { ConnectResult } from './connector-writer';
import { OAuthService } from './oauth.service';
import { APIKEY_PROVIDERS, OAUTH_PROVIDERS } from './providers';

class StartDto {
  @IsString()
  @IsNotEmpty()
  source!: string;
}

class CallbackDto {
  @IsString()
  @IsNotEmpty()
  state!: string;

  @IsString()
  @IsNotEmpty()
  code!: string;
}

class ApiKeyDto {
  @IsString()
  @IsNotEmpty()
  source!: string;

  @IsString()
  @IsNotEmpty()
  api_key!: string;

  @IsOptional()
  @IsString()
  base_url?: string;
}

@Controller('connectors')
@UseGuards(InternalGuard)
export class ConnectorsController {
  constructor(
    private readonly db: DatabaseService,
    private readonly oauth: OAuthService,
    private readonly apiKeys: ApiKeyService,
  ) {}

  private requireManager(principal: Principal): void {
    if (principal.role !== 'admin' && principal.role !== 'owner') {
      throw new ForbiddenException(
        'only an admin or owner can manage connectors',
      );
    }
  }

  @Get('available')
  async available(
    @CurrentPrincipal() principal: Principal,
  ): Promise<{ source: string; kind: string; status: string | null }[]> {
    const connected = await this.db.withTenant(
      principal.tenantId,
      async (c) => {
        const rows = await c.query<{ source_type: string; status: string }>(
          `select source_type, status from connectors`,
        );
        return new Map(rows.rows.map((r) => [r.source_type, r.status]));
      },
    );
    return [...OAUTH_PROVIDERS, ...APIKEY_PROVIDERS].map((p) => ({
      source: p.source,
      kind: p.kind,
      status: connected.get(p.source) ?? null,
    }));
  }

  @Post('oauth/start')
  async start(
    @CurrentPrincipal() principal: Principal,
    @Body() dto: StartDto,
  ): Promise<{ authorize_url: string }> {
    this.requireManager(principal);
    return this.oauth.start(principal.tenantId, principal.personId, dto.source);
  }

  @Post('oauth/callback')
  async callback(@Body() dto: CallbackDto): Promise<ConnectResult> {
    return this.oauth.callback(dto.state, dto.code);
  }

  @Post('apikey')
  async apikey(
    @CurrentPrincipal() principal: Principal,
    @Body() dto: ApiKeyDto,
  ): Promise<ConnectResult> {
    this.requireManager(principal);
    return this.apiKeys.store(
      principal.tenantId,
      dto.source,
      dto.api_key,
      dto.base_url ?? null,
    );
  }

  @Delete(':id')
  async disconnect(
    @CurrentPrincipal() principal: Principal,
    @Param('id', new ParseUUIDPipe({ errorHttpStatusCode: 404 })) id: string,
  ): Promise<{ disconnected: boolean }> {
    this.requireManager(principal);
    await this.db.withTenant(principal.tenantId, (c) =>
      c.query(
        `update connectors set status = 'archived', config = '{}'::jsonb where id = $1`,
        [id],
      ),
    );
    return { disconnected: true };
  }
}
