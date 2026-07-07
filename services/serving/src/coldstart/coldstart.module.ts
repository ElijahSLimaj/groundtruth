import { Module } from '@nestjs/common';

import { DriftModule } from '../drift/drift.module';
import { ColdStartService } from './coldstart.service';

@Module({
  imports: [DriftModule],
  providers: [ColdStartService],
  exports: [ColdStartService],
})
export class ColdStartModule {}
