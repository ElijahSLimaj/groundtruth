import { Module } from '@nestjs/common';

import { AuthModule } from '../auth/auth.module';
import { InternalGuard } from '../auth/internal.guard';
import { AccountController } from './account.controller';

@Module({
  imports: [AuthModule],
  controllers: [AccountController],
  providers: [InternalGuard],
})
export class AccountModule {}
