import { Module } from '@nestjs/common';
import { AppController } from './app.controller';
import { AppService } from './app.service';
import { AdminModule } from './admin/admin.module';
import { CanonModule } from './canon/canon.module';
import { ColdStartModule } from './coldstart/coldstart.module';
import { DatabaseModule } from './database/database.module';
import { DriftModule } from './drift/drift.module';
import { McpModule } from './mcp/mcp.module';
import { SchedulerModule } from './scheduler/scheduler.module';
import { SlackAppModule } from './slackapp/slackapp.module';

@Module({
  imports: [
    DatabaseModule,
    AdminModule,
    CanonModule,
    DriftModule,
    ColdStartModule,
    SlackAppModule,
    SchedulerModule,
    McpModule,
  ],
  controllers: [AppController],
  providers: [AppService],
})
export class AppModule {}
