import {
  Body,
  Controller,
  Get,
  Param,
  ParseUUIDPipe,
  Post,
  Query,
  UseGuards,
} from '@nestjs/common';

import { ApiKeyGuard } from '../auth/api-key.guard';
import { CurrentPrincipal } from '../auth/principal';
import type { Principal } from '../auth/principal';
import { CanonService } from './canon.service';
import { ProposeUpdateDto, QueryRequestDto } from './dto';

@Controller('tools')
@UseGuards(ApiKeyGuard)
export class CanonController {
  constructor(private readonly canon: CanonService) {}

  @Post('query')
  query(
    @CurrentPrincipal() principal: Principal,
    @Body() dto: QueryRequestDto,
  ) {
    return this.canon.query(principal, dto);
  }

  @Get('entries/:id')
  getEntry(
    @CurrentPrincipal() principal: Principal,
    @Param('id', new ParseUUIDPipe({ errorHttpStatusCode: 404 })) id: string,
  ) {
    return this.canon.getEntry(principal, id);
  }

  @Get('conflicts')
  listConflicts(
    @CurrentPrincipal() principal: Principal,
    @Query('domain') domain?: string,
  ) {
    return this.canon.listConflicts(principal, domain);
  }

  @Post('proposals')
  proposeUpdate(
    @CurrentPrincipal() principal: Principal,
    @Body() dto: ProposeUpdateDto,
  ) {
    return this.canon.proposeUpdate(principal, dto);
  }
}
