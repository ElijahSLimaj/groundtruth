import { Module } from '@nestjs/common';

import { BillingModule } from '../billing/billing.module';
import { ColdStartModule } from '../coldstart/coldstart.module';
import { ConnectorsModule } from '../connectors/connectors.module';
import { DriftModule } from '../drift/drift.module';
import { SlackAppModule } from '../slackapp/slackapp.module';
import { SchedulerService } from './scheduler.service';

@Module({
  imports: [
    DriftModule,
    ColdStartModule,
    SlackAppModule,
    ConnectorsModule,
    BillingModule,
  ],
  providers: [SchedulerService],
  exports: [SchedulerService],
})
export class SchedulerModule {}
