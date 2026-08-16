import {
  BadRequestException,
  Body,
  Controller,
  Post,
  Req,
  UseGuards,
} from '@nestjs/common';
import type { RawBodyRequest } from '@nestjs/common';
import type { Request } from 'express';
import { IsEmail, IsIn, IsNotEmpty, IsString } from 'class-validator';

import { InternalGuard } from '../auth/internal.guard';
import { InternalSecretGuard } from '../auth/internal-secret.guard';
import { CurrentPrincipal } from '../auth/principal';
import type { Principal } from '../auth/principal';
import { BillingService } from './billing.service';

class CheckoutDto {
  @IsEmail()
  email!: string;

  @IsString()
  @IsNotEmpty()
  name!: string;

  @IsString()
  @IsNotEmpty()
  company!: string;

  @IsIn(['core', 'growth', 'scale'])
  plan!: string;

  @IsIn(['month', 'year'])
  interval!: 'month' | 'year';
}

@Controller('billing')
export class BillingController {
  constructor(private readonly billing: BillingService) {}

  @Post('checkout')
  @UseGuards(InternalSecretGuard)
  async checkout(@Body() dto: CheckoutDto): Promise<{ url: string }> {
    return this.billing.createCheckout(dto);
  }

  @Post('portal')
  @UseGuards(InternalGuard)
  async portal(
    @CurrentPrincipal() principal: Principal,
  ): Promise<{ url: string }> {
    return this.billing.createPortal(principal.tenantId);
  }

  @Post('webhook')
  async webhook(
    @Req() request: RawBodyRequest<Request>,
  ): Promise<{ received: boolean }> {
    const signature = String(request.headers['stripe-signature'] ?? '');
    if (!request.rawBody || !signature) {
      throw new BadRequestException('missing raw body or stripe signature');
    }
    await this.billing.handleWebhook(request.rawBody, signature);
    return { received: true };
  }
}
