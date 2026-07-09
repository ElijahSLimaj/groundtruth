'use server';

import { revalidatePath } from 'next/cache';
import { PoolClient } from 'pg';

import { withTenant } from '../../../lib/db';
import { requireViewer } from '../../../lib/session';

interface ProposalRow {
  id: string;
  entry_id: string | null;
  pending_version_id: string | null;
  drafted_statement: string;
  drafted_attributes: Record<string, unknown>;
  domain: string | null;
  routed_to: string;
  status: string;
}

async function loadProposal(
  client: PoolClient,
  proposalId: string,
): Promise<ProposalRow> {
  const rows = await client.query<ProposalRow>(
    `select id, entry_id, pending_version_id, drafted_statement,
            drafted_attributes, domain, routed_to, status
     from drift_proposals where id = $1`,
    [proposalId],
  );
  const proposal = rows.rows[0];
  if (!proposal || proposal.status !== 'pending') {
    throw new Error('proposal not found or already resolved');
  }
  return proposal;
}

async function evidenceEventIds(
  client: PoolClient,
  proposalId: string,
): Promise<string[]> {
  const rows = await client.query<{ event_id: string }>(
    `select distinct event_id from drift_evidence where proposal_id = $1`,
    [proposalId],
  );
  return rows.rows.map((r) => r.event_id);
}

export async function approveProposal(formData: FormData): Promise<void> {
  const proposalId = String(formData.get('proposal_id') ?? '');
  if (!proposalId) {
    throw new Error('proposal_id is required');
  }
  const viewer = await requireViewer();

  await withTenant(viewer.tenantId, async (client) => {
    const proposal = await loadProposal(client, proposalId);
    let versionId = proposal.pending_version_id;

    if (!versionId && proposal.entry_id) {
      const events = await evidenceEventIds(client, proposalId);
      const created = await client.query<{ canon_submit_version: string }>(
        `select public.canon_submit_version($1, $2, $3, $4, $5, $6)`,
        [
          proposal.entry_id,
          viewer.personId,
          proposal.drafted_statement,
          JSON.stringify(proposal.drafted_attributes ?? {}),
          events,
          proposalId,
        ],
      );
      versionId = created.rows[0].canon_submit_version;
    }

    if (!versionId && !proposal.entry_id) {
      if (!proposal.domain) {
        throw new Error('proposal has no domain to draft an entry into');
      }
      const events = await evidenceEventIds(client, proposalId);
      const draft = await client.query<{
        entry_id: string;
        version_id: string;
      }>(
        `select * from public.canon_create_entry_draft($1, 'operational', $2, $3, $4, '{"scope": "tenant"}', interval '60 days', $5)`,
        [
          proposal.domain,
          proposal.routed_to,
          proposal.drafted_statement,
          JSON.stringify(proposal.drafted_attributes ?? {}),
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
      viewer.personId,
      'approved from the web queue',
    ]);
  });

  revalidatePath('/queue');
  revalidatePath('/canon');
  revalidatePath('/drift');
}

export async function rejectProposal(formData: FormData): Promise<void> {
  const proposalId = String(formData.get('proposal_id') ?? '');
  const reason = String(formData.get('reason') ?? 'other');
  const note = String(formData.get('note') ?? '') || null;
  if (!proposalId) {
    throw new Error('proposal_id is required');
  }
  const viewer = await requireViewer();

  await withTenant(viewer.tenantId, async (client) => {
    const proposal = await loadProposal(client, proposalId);
    if (proposal.pending_version_id) {
      await client.query(`select public.canon_reject($1, $2, $3, $4)`, [
        proposal.pending_version_id,
        viewer.personId,
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
      [
        viewer.tenantId,
        viewer.personId,
        proposalId,
        JSON.stringify({ reason, note }),
      ],
    );
  });

  revalidatePath('/queue');
  revalidatePath('/drift');
}
