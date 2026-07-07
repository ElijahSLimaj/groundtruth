import { Module } from '@nestjs/common';
import { AppController } from './app.controller';
import { AppService } from './app.service';
import { CanonModule } from './canon/canon.module';
import { DatabaseModule } from './database/database.module';
import { DriftModule } from './drift/drift.module';

@Module({
  imports: [DatabaseModule, CanonModule, DriftModule],
  controllers: [AppController],
  providers: [AppService],
})
export class AppModule {}
