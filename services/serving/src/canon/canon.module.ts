import { Module } from '@nestjs/common';

import { AuthModule } from '../auth/auth.module';
import { CanonController } from './canon.controller';
import { CanonService } from './canon.service';

@Module({
  imports: [AuthModule],
  controllers: [CanonController],
  providers: [CanonService],
})
export class CanonModule {}
