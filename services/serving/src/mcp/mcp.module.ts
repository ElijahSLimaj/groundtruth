import { Module } from '@nestjs/common';

import { AuthModule } from '../auth/auth.module';
import { CanonModule } from '../canon/canon.module';
import { McpController } from './mcp.controller';

@Module({
  imports: [AuthModule, CanonModule],
  controllers: [McpController],
})
export class McpModule {}
