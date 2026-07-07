import { createParamDecorator, ExecutionContext } from '@nestjs/common';

export interface Principal {
  keyId: string;
  tenantId: string;
  personId: string;
  role: string;
  displayName: string;
  allowedDomains: string[] | null;
  rateTier: string;
  principals: string[];
}

export interface AuthenticatedRequest {
  principal?: Principal;
  headers: Record<string, string | string[] | undefined>;
  url?: string;
}

export const CurrentPrincipal = createParamDecorator(
  (_data: unknown, context: ExecutionContext): Principal => {
    const request = context.switchToHttp().getRequest<AuthenticatedRequest>();
    if (!request.principal) {
      throw new Error('no principal on request, is the ApiKeyGuard applied?');
    }
    return request.principal;
  },
);
