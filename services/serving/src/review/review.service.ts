import { Injectable } from '@nestjs/common';
import { PoolClient } from 'pg';

import { DatabaseService } from '../database/database.service';

export interface ReviewableProposal {
  id: string;
  tenantId: string;
  entryId: string | null;
  pendingVersionId: string | null;
  draftedStatement: string;
  draftedAttributes: Record<string, unknown>;
  currentStatement: string | null;
  domain: string | null;
  kind: string;
  confidence: number;
  routedTo: string;
  strategic: boolean;
}

const REJECT_REASONS = new Set([
  'wrong',
  'duplicate',
  'not_canon_worthy',
  'bad_draft',
  'other',
]);

@Injectable()
export class ReviewService {
  constructor(private readonly db: DatabaseService) {}

  async approve(
    tenantId: string,
    proposalId: string,
    reviewerId: string,
    note: string | null,
  ): Promise<void> {
    await this.db.withTenant(tenantId, async (client) => {
      const proposal = await this.loadPending(client, proposalId);
      let versionId = proposal.pendingVersionId;

      if (!versionId && proposal.entryId) {
        const events = await this.evidenceEvents(client, proposalId);
        const created = await client.query<{ canon_submit_version: string }>(
          `select public.canon_submit_version($1, $2, $3, $4, $5, $6)`,
          [
            proposal.entryId,
            reviewerId,
            proposal.draftedStatement,
            JSON.stringify(proposal.draftedAttributes),
            events,
            proposalId,
          ],
        );
        versionId = created.rows[0].canon_submit_version;
      }
      if (!versionId && !proposal.entryId) {
        if (!proposal.domain) {
          throw new Error('proposal has no domain to draft an entry into');
        }
        const events = await this.evidenceEvents(client, proposalId);
        const draft = await client.query<{
          entry_id: string;
          version_id: string;
        }>(
          `select * from public.canon_create_entry_draft($1, 'operational', $2, $3, $4, '{"scope": "tenant"}', interval '60 days', $5)`,
          [
            proposal.domain,
            proposal.routedTo,
            proposal.draftedStatement,
            JSON.stringify(proposal.draftedAttributes),
            events,
          ],
        );
        versionId = draft.rows[0].version_id;
        await client.query(
          `update drift_proposals set entry_id = $2, pending_version_id = $3 where id = $1`,
          [proposalId, draft.rows[0].entry_id, versionId],
        );
      }

      await client.query(`select public.canon_approve($1, $2, $3)`, [
        versionId,
        reviewerId,
        note,
      ]);
    });
  }

  async reject(
    tenantId: string,
    proposalId: string,
    reviewerId: string,
    reason: string,
    note: string | null,
  ): Promise<void> {
    if (!REJECT_REASONS.has(reason)) {
      throw new Error(`invalid rejection reason ${reason}`);
    }
    await this.db.withTenant(tenantId, async (client) => {
      const proposal = await this.loadPending(client, proposalId);
      if (proposal.pendingVersionId) {
        await client.query(`select public.canon_reject($1, $2, $3, $4)`, [
          proposal.pendingVersionId,
          reviewerId,
          reason,
          note,
        ]);
        return;
      }
      await client.query(
        `update drift_proposals
         set status = 'resolved', resolution = $2, resolution_note = $3, resolved_at = now()
         where id = $1`,
        [proposalId, reason, note],
      );
      await client.query(
        `insert into audit_log (tenant_id, actor_id, action, subject_type, subject_id, detail)
         values ($1, $2, 'drift.proposal.rejected', 'drift_proposal', $3, $4)`,
        [tenantId, reviewerId, proposalId, JSON.stringify({ reason, note })],
      );
    });
  }

  private async loadPending(
    client: PoolClient,
    proposalId: string,
  ): Promise<ReviewableProposal> {
    const rows = await client.query<{
      id: string;
      tenant_id: string;
      entry_id: string | null;
      pending_version_id: string | null;
      drafted_statement: string;
      drafted_attributes: Record<string, unknown> | null;
      current_statement: string | null;
      domain: string | null;
      kind: string;
      confidence: string;
      routed_to: string;
      strategic: boolean;
      status: string;
    }>(
      `select dp.id, dp.tenant_id, dp.entry_id, dp.pending_version_id,
              dp.drafted_statement, dp.drafted_attributes,
              cv.statement as current_statement, coalesce(ce.domain, dp.domain) as domain,
              dp.kind, dp.confidence, dp.routed_to, dp.strategic, dp.status
       from drift_proposals dp
       left join canon_entries ce on ce.id = dp.entry_id
       left join canon_versions cv on cv.id = ce.current_version_id
       where dp.id = $1`,
      [proposalId],
    );
    const row = rows.rows[0];
    if (!row || row.status !== 'pending') {
      throw new Error('proposal not found or already resolved');
    }
    return {
      id: row.id,
      tenantId: row.tenant_id,
      entryId: row.entry_id,
      pendingVersionId: row.pending_version_id,
      draftedStatement: row.drafted_statement,
      draftedAttributes: row.drafted_attributes ?? {},
      currentStatement: row.current_statement,
      domain: row.domain,
      kind: row.kind,
      confidence: Number(row.confidence),
      routedTo: row.routed_to,
      strategic: row.strategic,
    };
  }

  private async evidenceEvents(
    client: PoolClient,
    proposalId: string,
  ): Promise<string[]> {
    const rows = await client.query<{ event_id: string }>(
      `select distinct event_id from drift_evidence where proposal_id = $1`,
      [proposalId],
    );
    return rows.rows.map((r) => r.event_id);
  }
}
