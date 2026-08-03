import { Module } from '@nestjs/common';

import { AuthModule } from '../auth/auth.module';
import { InternalGuard } from '../auth/internal.guard';
import { ApiKeyService } from './apikey.service';
import { ConnectorsController } from './connectors.controller';
import { OAuthService } from './oauth.service';

@Module({
  imports: [AuthModule],
  controllers: [ConnectorsController],
  providers: [OAuthService, ApiKeyService, InternalGuard],
  exports: [OAuthService],
})
export class ConnectorsModule {}
