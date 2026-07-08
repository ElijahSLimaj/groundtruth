import { Module } from '@nestjs/common';

import { AuthModule } from '../auth/auth.module';
import { AdminController } from './admin.controller';
import { ErasureController, TombstoneController } from './erasure.controller';
import { ErasureService } from './erasure.service';

@Module({
  imports: [AuthModule],
  controllers: [AdminController, ErasureController, TombstoneController],
  providers: [ErasureService],
})
export class AdminModule {}
