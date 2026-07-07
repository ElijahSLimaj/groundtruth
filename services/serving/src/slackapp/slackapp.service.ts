import { Inject, Injectable, Logger } from '@nestjs/common';

import { DatabaseService } from '../database/database.service';
import { SERVING_CONFIG } from '../config';
import type { ServingConfig } from '../config';
import { ReviewService } from '../review/review.service';
import { SLACK_WEB_API } from './slack-web';
import type { SlackWebApi } from './slack-web';

interface NotifiableProposal {
  id: string;
  tenant_id: string;
  kind: string;
  confidence: string;
  domain: string | null;
  drafted_statement: string;
  current_statement: string | null;
  strategic: boolean;
}

export function proposalBlocks(proposal: {
  id: string;
  kind: string;
  confidence: number;
  domain: string | null;
  draftedStatement: string;
  currentStatement: string | null;
  strategic: boolean;
}): unknown[] {
  const eyebrow = [
    proposal.kind,
    proposal.domain ?? 'no domain',
    `${Math.round(proposal.confidence * 100)}% confidence`,
    proposal.strategic ? 'STRATEGIC' : null,
  ]
    .filter(Boolean)
    .join(' · ');

  return [
    {
      type: 'section',
      text: { type: 'mrkdwn', text: `*${eyebrow}*` },
    },
    {
      type: 'section',
      text: {
        type: 'mrkdwn',
        text: proposal.currentStatement
          ? `> ~${proposal.currentStatement}~\n> *${proposal.draftedStatement}*`
          : `> *${proposal.draftedStatement}*`,
      },
    },
    {
      type: 'actions',
      elements: [
        {
          type: 'button',
          style: 'primary',
          text: { type: 'plain_text', text: 'Approve' },
          action_id: 'approve_proposal',
          value: proposal.id,
        },
        {
          type: 'button',
          text: { type: 'plain_text', text: 'Edit' },
          action_id: 'edit_proposal',
          value: proposal.id,
          url: undefined,
        },
        {
          type: 'button',
          style: 'danger',
          text: { type: 'plain_text', text: 'Reject' },
          action_id: 'reject_proposal',
          value: proposal.id,
        },
      ],
    },
  ];
}

@Injectable()
export class SlackAppService {
  private readonly logger = new Logger(SlackAppService.name);

  constructor(
    private readonly db: DatabaseService,
    private readonly review: ReviewService,
    @Inject(SLACK_WEB_API) private readonly slack: SlackWebApi,
    @Inject(SERVING_CONFIG) private readonly config: ServingConfig,
  ) {}

  get enabled(): boolean {
    return Boolean(
      this.config.slackBotToken &&
      this.config.slackSigningSecret &&
      this.config.slackApprovalChannel,
    );
  }

  async notifyPending(tenantId: string): Promise<number> {
    if (!this.enabled) {
      return 0;
    }
    const proposals = await this.db.withTenant(tenantId, async (client) => {
      const rows = await client.query<NotifiableProposal>(
        `select dp.id, dp.tenant_id, dp.kind, dp.confidence,
                coalesce(ce.domain, dp.domain) as domain,
                dp.drafted_statement, cv.statement as current_statement, dp.strategic
         from drift_proposals dp
         left join canon_entries ce on ce.id = dp.entry_id
         left join canon_versions cv on cv.id = ce.current_version_id
         where dp.status = 'pending' and dp.slack_notified_at is null
         order by dp.created_at
         limit 20`,
      );
      return rows.rows;
    });

    let sent = 0;
    for (const proposal of proposals) {
      await this.slack.postMessage({
        channel: this.config.slackApprovalChannel as string,
        text: `Proposal: ${proposal.drafted_statement}`,
        blocks: proposalBlocks({
          id: proposal.id,
          kind: proposal.kind,
          confidence: Number(proposal.confidence),
          domain: proposal.domain,
          draftedStatement: proposal.drafted_statement,
          currentStatement: proposal.current_statement,
          strategic: proposal.strategic,
        }),
      });
      await this.db.withTenant(tenantId, (client) =>
        client.query(
          `update drift_proposals set slack_notified_at = now() where id = $1`,
          [proposal.id],
        ),
      );
      sent++;
    }
    return sent;
  }

  async handleAction(input: {
    tenantId: string;
    slackUserId: string;
    actionId: string;
    proposalId: string;
  }): Promise<string> {
    const reviewer = await this.db.withTenant(
      input.tenantId,
      async (client) => {
        const rows = await client.query<{ id: string; display_name: string }>(
          `select id, display_name from people where slack_user_id = $1`,
          [input.slackUserId],
        );
        return rows.rows[0];
      },
    );
    if (!reviewer) {
      return 'Your Slack account is not linked to a Company Brain person. Ask an admin to set your slack_user_id.';
    }

    if (input.actionId === 'approve_proposal') {
      await this.review.approve(
        input.tenantId,
        input.proposalId,
        reviewer.id,
        'approved from slack',
      );
      return `Approved by ${reviewer.display_name}.`;
    }
    if (input.actionId === 'reject_proposal') {
      await this.review.reject(
        input.tenantId,
        input.proposalId,
        reviewer.id,
        'other',
        'rejected from slack',
      );
      return `Rejected by ${reviewer.display_name}.`;
    }
    if (input.actionId === 'edit_proposal') {
      return 'Open the web queue to edit before approving.';
    }
    this.logger.warn(
      JSON.stringify({ event: 'slack_unknown_action', action: input.actionId }),
    );
    return 'Unknown action.';
  }
}
