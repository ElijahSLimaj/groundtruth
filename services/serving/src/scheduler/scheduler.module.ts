import { Module } from '@nestjs/common';

import { ColdStartModule } from '../coldstart/coldstart.module';
import { DriftModule } from '../drift/drift.module';
import { SlackAppModule } from '../slackapp/slackapp.module';
import { SchedulerService } from './scheduler.service';

@Module({
  imports: [DriftModule, ColdStartModule, SlackAppModule],
  providers: [SchedulerService],
  exports: [SchedulerService],
})
export class SchedulerModule {}
