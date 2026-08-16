import { timingSafeEqual } from 'node:crypto';

import {
  CanActivate,
  ExecutionContext,
  Inject,
  Injectable,
  UnauthorizedException,
} from '@nestjs/common';

import { SERVING_CONFIG } from '../config';
import type { ServingConfig } from '../config';
import type { AuthenticatedRequest } from './principal';

@Injectable()
export class InternalSecretGuard implements CanActivate {
  constructor(@Inject(SERVING_CONFIG) private readonly config: ServingConfig) {}

  canActivate(context: ExecutionContext): boolean {
    const request = context.switchToHttp().getRequest<AuthenticatedRequest>();
    const secret = this.config.internalApiSecret;
    const provided = String(request.headers['x-internal-secret'] ?? '');
    if (!secret || provided.length !== secret.length) {
      throw new UnauthorizedException('internal access denied');
    }
    if (!timingSafeEqual(Buffer.from(provided), Buffer.from(secret))) {
      throw new UnauthorizedException('internal access denied');
    }
    return true;
  }
}
