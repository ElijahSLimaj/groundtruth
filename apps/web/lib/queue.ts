import { withTenant } from './db';
import type { Viewer } from './session';

export interface QueueProposal {
  id: string;
  kind: string;
  status: string;
  confidence: number;
  domain: string | null;
  entryTier: string | null;
  strategic: boolean;
  recurring: boolean;
  origin: string;
  conflictingField: string | null;
  draftedStatement: string;
  draftedAttributes: Record<string, unknown>;
  currentStatement: string | null;
  currentAttributes: Record<string, unknown>;
  entryId: string | null;
  pendingVersionId: string | null;
  routedTo: string;
  createdAt: Date;
  evidence: { content: string; occurredAt: Date | null }[];
  withheldEvidence: { count: number; sourceTypes: string[] };
}

export async function loadQueue(viewer: Viewer): Promise<{
  pending: QueueProposal[];
  queuedCount: number;
}> {
  return withTenant(viewer.tenantId, async (client) => {
    interface Row {
      id: string;
      kind: string;
      status: string;
      confidence: string;
      domain: string | null;
      entry_tier: string | null;
      strategic: boolean;
      recurring_after_rejection: boolean;
      origin: string;
      conflicting_field: string | null;
      drafted_statement: string;
      drafted_attributes: Record<string, unknown>;
      current_statement: string | null;
      current_attributes: Record<string, unknown> | null;
      entry_id: string | null;
      pending_version_id: string | null;
      routed_to: string;
      created_at: Date;
    }
    const isAdmin = viewer.role === 'admin';
    const rows = await client.query<Row>(
      `
      select dp.id, dp.kind, dp.status, dp.confidence, coalesce(ce.domain, dp.domain) as domain,
             ce.tier as entry_tier, dp.strategic, dp.recurring_after_rejection, dp.origin,
             dp.conflicting_field, dp.drafted_statement, dp.drafted_attributes,
             cv.statement as current_statement, cv.attributes as current_attributes,
             dp.entry_id, dp.pending_version_id, dp.routed_to, dp.created_at
      from drift_proposals dp
      left join canon_entries ce on ce.id = dp.entry_id
      left join canon_versions cv on cv.id = ce.current_version_id
      where dp.status in ('pending', 'queued')
        and ($2 or dp.routed_to = $1 or dp.escalated_to = $1)
      order by dp.strategic desc, dp.confidence desc, dp.created_at asc
      `,
      [viewer.personId, isAdmin],
    );

    const pendingRows = rows.rows.filter((r) => r.status === 'pending');
    const queuedCount = rows.rows.length - pendingRows.length;

    const principals = [`person:${viewer.personId}`];
    const proposals: QueueProposal[] = [];
    for (const row of pendingRows) {
      const evidence = await client.query<{
        content: string;
        occurred_at: Date | null;
      }>(
        `select c.content, c.event_occurred_at as occurred_at
         from drift_evidence de
         join event_chunks c on c.id = de.chunk_id
         where de.proposal_id = $1
           and public.acl_admits(c.acl, $2)
         order by de.added_at
         limit 5`,
        [row.id, principals],
      );
      const withheld = await client.query<{
        count: string;
        source_types: string[];
      }>(
        `select count(*)::text as count,
                coalesce(array_agg(distinct c.source_type), '{}') as source_types
         from drift_evidence de
         join event_chunks c on c.id = de.chunk_id
         where de.proposal_id = $1
           and not public.acl_admits(c.acl, $2)`,
        [row.id, principals],
      );
      proposals.push({
        id: row.id,
        kind: row.kind,
        status: row.status,
        confidence: Number(row.confidence),
        domain: row.domain,
        entryTier: row.entry_tier,
        strategic: row.strategic,
        recurring: row.recurring_after_rejection,
        origin: row.origin,
        conflictingField: row.conflicting_field,
        draftedStatement: row.drafted_statement,
        draftedAttributes: row.drafted_attributes ?? {},
        currentStatement: row.current_statement,
        currentAttributes: row.current_attributes ?? {},
        entryId: row.entry_id,
        pendingVersionId: row.pending_version_id,
        routedTo: row.routed_to,
        createdAt: row.created_at,
        evidence: evidence.rows.map((e) => ({
          content: e.content,
          occurredAt: e.occurred_at,
        })),
        withheldEvidence: {
          count: Number(withheld.rows[0]?.count ?? 0),
          sourceTypes: withheld.rows[0]?.source_types ?? [],
        },
      });
    }
    return { pending: proposals, queuedCount };
  });
}
