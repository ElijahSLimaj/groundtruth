import { Module } from '@nestjs/common';

import { SERVING_CONFIG } from '../config';
import type { ServingConfig } from '../config';
import { ReviewService } from '../review/review.service';
import { SlackAppController } from './slackapp.controller';
import { SlackAppService } from './slackapp.service';
import {
  DisabledSlackWebApi,
  HttpSlackWebApi,
  SLACK_WEB_API,
} from './slack-web';

@Module({
  controllers: [SlackAppController],
  providers: [
    {
      provide: SLACK_WEB_API,
      inject: [SERVING_CONFIG],
      useFactory: (config: ServingConfig) =>
        config.slackBotToken
          ? new HttpSlackWebApi(config.slackBotToken)
          : new DisabledSlackWebApi(),
    },
    ReviewService,
    SlackAppService,
  ],
  exports: [SlackAppService, ReviewService],
})
export class SlackAppModule {}
