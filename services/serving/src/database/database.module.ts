import { Global, Module } from '@nestjs/common';

import { SERVING_CONFIG, loadConfig } from '../config';
import { DatabaseService } from './database.service';

@Global()
@Module({
  providers: [
    { provide: SERVING_CONFIG, useFactory: () => loadConfig() },
    DatabaseService,
  ],
  exports: [SERVING_CONFIG, DatabaseService],
})
export class DatabaseModule {}
