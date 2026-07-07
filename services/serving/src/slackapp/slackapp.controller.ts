import {
  Controller,
  Headers,
  HttpCode,
  NotFoundException,
  Post,
  Req,
  UnauthorizedException,
} from '@nestjs/common';
import type { RawBodyRequest } from '@nestjs/common';
import type { Request } from 'express';
import { Inject } from '@nestjs/common';

import { SERVING_CONFIG } from '../config';
import type { ServingConfig } from '../config';
import { verifySlackSignature } from './signature';
import { SlackAppService } from './slackapp.service';

interface BlockActionsPayload {
  type: string;
  user: { id: string };
  actions: { action_id: string; value: string }[];
}

@Controller('slack')
export class SlackAppController {
  constructor(
    private readonly slackApp: SlackAppService,
    @Inject(SERVING_CONFIG) private readonly config: ServingConfig,
  ) {}

  @Post('interactions')
  @HttpCode(200)
  async interactions(
    @Req() request: RawBodyRequest<Request>,
    @Headers('x-slack-signature') signature = '',
    @Headers('x-slack-request-timestamp') timestamp = '',
  ): Promise<{ response_type: string; text: string }> {
    if (!this.slackApp.enabled || !this.config.slackTenantId) {
      throw new NotFoundException();
    }
    const rawBody = request.rawBody?.toString('utf8') ?? '';
    const valid = verifySlackSignature({
      signingSecret: this.config.slackSigningSecret as string,
      timestamp,
      rawBody,
      signature,
    });
    if (!valid) {
      throw new UnauthorizedException('invalid slack signature');
    }

    const params = new URLSearchParams(rawBody);
    const payloadRaw = params.get('payload');
    if (!payloadRaw) {
      return { response_type: 'ephemeral', text: 'Nothing to do.' };
    }
    const payload = JSON.parse(payloadRaw) as BlockActionsPayload;
    const action = payload.actions?.[0];
    if (payload.type !== 'block_actions' || !action) {
      return { response_type: 'ephemeral', text: 'Nothing to do.' };
    }

    const text = await this.slackApp.handleAction({
      tenantId: this.config.slackTenantId,
      slackUserId: payload.user.id,
      actionId: action.action_id,
      proposalId: action.value,
    });
    return { response_type: 'ephemeral', text };
  }
}
