import { Module } from '@nestjs/common';
import { AppController } from './app.controller';
import { AppService } from './app.service';
import { AccountModule } from './account/account.module';
import { AdminModule } from './admin/admin.module';
import { BillingModule } from './billing/billing.module';
import { CanonModule } from './canon/canon.module';
import { ChatModule } from './chat/chat.module';
import { ColdStartModule } from './coldstart/coldstart.module';
import { ConnectorsModule } from './connectors/connectors.module';
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
    ChatModule,
    ConnectorsModule,
    BillingModule,
    AccountModule,
  ],
  controllers: [AppController],
  providers: [AppService],
})
export class AppModule {}
