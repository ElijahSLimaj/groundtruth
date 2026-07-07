import { Module } from '@nestjs/common';
import { AppController } from './app.controller';
import { AppService } from './app.service';
import { CanonModule } from './canon/canon.module';
import { DatabaseModule } from './database/database.module';

@Module({
  imports: [DatabaseModule, CanonModule],
  controllers: [AppController],
  providers: [AppService],
})
export class AppModule {}
