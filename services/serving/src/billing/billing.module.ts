import { Module } from '@nestjs/common';

import { AuthModule } from '../auth/auth.module';
import { InternalGuard } from '../auth/internal.guard';
import { InternalSecretGuard } from '../auth/internal-secret.guard';
import { BillingController } from './billing.controller';
import { BillingService } from './billing.service';

@Module({
  imports: [AuthModule],
  controllers: [BillingController],
  providers: [BillingService, InternalGuard, InternalSecretGuard],
  exports: [BillingService],
})
export class BillingModule {}
