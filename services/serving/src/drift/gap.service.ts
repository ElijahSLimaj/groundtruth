import { Inject, Injectable, Logger } from '@nestjs/common';
import { PoolClient } from 'pg';

import { SERVING_CONFIG } from '../config';
import type { ServingConfig } from '../config';
import { DatabaseService } from '../database/database.service';
import { LLM_CLIENT } from './llm';
import type { LlmClient } from './llm';
import {
  chunkIdsBySourceId,
  EvidenceBlock,
  GAP_TIER2_PROMPT_VERSION,
  GAP_TIER2_SYSTEM,
  GAP_TIER3_PROMPT_VERSION,
  GAP_TIER3_SYSTEM,
  gapTier2UserPrompt,
  gapTier3UserPrompt,
} from './prompts';
import {
  attributeExcerpts,
  decodeGapTier3,
  GapTier2Result,
  GapTier3Wire,
  resolveTuning,
  TuningParams,
} from './schemas';

export interface GapRunResult {
  disabled: boolean;
  clusters: number;
  evidenceAttached: number;
  notCanonWorthy: number;
  invalidDrafts: number;
  proposalsCreated: number;
}

interface ClusterChunk {
  chunkId: string;
  eventId: string;
  authorKey: string;
  content: string;
}

const emptyResult = (disabled: boolean): GapRunResult => ({
  disabled,
  clusters: 0,
  evidenceAttached: 0,
  notCanonWorthy: 0,
  invalidDrafts: 0,
  proposalsCreated: 0,
});

@Injectable()
export class GapService {
  private readonly logger = new Logger(GapService.name);

  constructor(
    private readonly db: DatabaseService,
    @Inject(LLM_CLIENT) private readonly llm: LlmClient,
    @Inject(SERVING_CONFIG) private readonly config: ServingConfig,
  ) {}

  async runOnce(tenantId: string): Promise<GapRunResult> {
    if (!this.llm.enabled) {
      return emptyResult(true);
    }
    const result = emptyResult(false);

    const { tuning, clusters, domains } = await this.db.withTenant(
      tenantId,
      async (client) => {
        const tuningRow = await client.query<{ params: unknown }>(
          `select params from drift_tuning where tenant_id = public.app_tenant_id()`,
        );
        const resolved = resolveTuning(tuningRow.rows[0]?.params);
        await client.query(
          `delete from unmatched_chunks
           where added_at <= now() - make_interval(days => $1)`,
          [resolved.gap_buffer_days],
        );
        const rows = await client.query<{
          cluster_root: string;
          chunk_id: string;
          event_id: string;
          author_key: string;
          content: string;
        }>(`select * from public.gap_clusters($1, $2, $3, $4)`, [
          resolved.gap_similarity,
          resolved.gap_cluster_min_size,
          resolved.gap_min_authors,
          resolved.gap_buffer_days,
        ]);
        const domainRows = await client.query<{ domain: string }>(
          `select distinct domain from domain_schemas order by domain`,
        );
        const grouped = new Map<string, ClusterChunk[]>();
        for (const row of rows.rows) {
          const bucket = grouped.get(row.cluster_root) ?? [];
          bucket.push({
            chunkId: row.chunk_id,
            eventId: row.event_id,
            authorKey: row.author_key,
            content: row.content,
          });
          grouped.set(row.cluster_root, bucket);
        }
        return {
          tuning: resolved,
          clusters: grouped,
          domains: domainRows.rows.map((r) => r.domain),
        };
      },
    );
    result.clusters = clusters.size;

    for (const chunks of clusters.values()) {
      await this.evaluateCluster(tenantId, chunks, domains, tuning, result);
    }
    return result;
  }

  private async evaluateCluster(
    tenantId: string,
    chunks: ClusterChunk[],
    domains: string[],
    tuning: TuningParams,
    result: GapRunResult,
  ): Promise<void> {
    const chunkIds = chunks.map((c) => c.chunkId);
    const attached = await this.db.withTenant(tenantId, (client) =>
      this.attachToOpenProposal(client, tenantId, chunks),
    );
    if (attached !== null) {
      result.evidenceAttached += attached;
      return;
    }

    const tier2 = await this.llm.completeJson(
      {
        model: this.config.driftTier2Model,
        system: GAP_TIER2_SYSTEM,
        user: gapTier2UserPrompt({
          digest: chunks.slice(0, 8).map((c) => c.content),
          domains,
        }),
        maxTokens: 256,
        promptVersion: GAP_TIER2_PROMPT_VERSION,
      },
      GapTier2Result,
    );
    if (!tier2.canon_worthy || tier2.confidence < tuning.tier2_gate) {
      result.notCanonWorthy++;
      await this.dropFromBuffer(tenantId, chunkIds);
      return;
    }

    const attachedByDomain = await this.db.withTenant(
      tenantId,
      async (client) => {
        const open = await client.query<{ id: string }>(
          `select id from drift_proposals
           where kind = 'gap' and domain = $1 and status in ('pending', 'queued')
           limit 1`,
          [tier2.domain],
        );
        if (!open.rows[0]) {
          return null;
        }
        return this.attachEvidence(client, tenantId, open.rows[0].id, chunks);
      },
    );
    if (attachedByDomain !== null) {
      result.evidenceAttached += attachedByDomain;
      return;
    }

    const blocks: EvidenceBlock[] = chunks.map((chunk, index) => ({
      id: `e${index + 1}`,
      chunkId: chunk.chunkId,
      content: chunk.content,
    }));

    const wire = await this.llm.completeJson(
      {
        model: this.config.driftTier3Model,
        system: GAP_TIER3_SYSTEM,
        user: gapTier3UserPrompt({
          domain: tier2.domain,
          evidence: blocks,
        }),
        maxTokens: 2048,
        promptVersion: GAP_TIER3_PROMPT_VERSION,
      },
      GapTier3Wire,
    );
    const draft = decodeGapTier3(wire);
    if (!draft) {
      result.invalidDrafts++;
      this.logger.warn(
        JSON.stringify({
          event: 'gap_invalid_draft',
          tenant: tenantId,
          cluster_size: chunks.length,
        }),
      );
      return;
    }

    await this.db.withTenant(tenantId, async (client) => {
      const routedTo = await this.routeTarget(client, tier2.domain);
      if (!routedTo) {
        this.logger.warn(
          JSON.stringify({
            event: 'gap_no_route_target',
            tenant: tenantId,
            domain: tier2.domain,
          }),
        );
        return;
      }
      const budgetUsed = await client.query<{ n: string }>(
        `select count(*) as n from drift_proposals
         where routed_to = $1 and status = 'pending'
           and created_at > now() - interval '7 days'`,
        [routedTo],
      );
      const status =
        Number(budgetUsed.rows[0].n) >= tuning.owner_weekly_budget
          ? 'queued'
          : 'pending';

      const inserted = await client.query<{ id: string }>(
        `insert into drift_proposals
           (tenant_id, kind, drafted_statement, drafted_attributes,
            confidence, routed_to, domain, origin, status)
         values ($1, 'gap', $2, $3, $4, $5, $6, 'drift_engine', $7)
         returning id`,
        [
          tenantId,
          draft.draftedStatement,
          JSON.stringify(draft.draftedAttributes),
          draft.confidence,
          routedTo,
          tier2.domain,
          status,
        ],
      );
      const proposalId = inserted.rows[0].id;
      for (const chunk of chunks) {
        await client.query(
          `insert into drift_evidence (tenant_id, proposal_id, event_id, chunk_id)
           values ($1, $2, $3, $4)`,
          [tenantId, proposalId, chunk.eventId, chunk.chunkId],
        );
      }
      await client.query(
        `insert into audit_log (tenant_id, actor_id, action, subject_type, subject_id, detail)
         values ($1, null, 'drift.proposal.created', 'drift_proposal', $2, $3)`,
        [
          tenantId,
          proposalId,
          JSON.stringify({
            kind: 'gap',
            domain: tier2.domain,
            cluster_size: chunks.length,
            confidence: draft.confidence,
            description: draft.gapDescription,
            excerpts: attributeExcerpts(
              draft.supportingExcerpts,
              chunkIdsBySourceId(blocks),
            ).map((excerpt) => ({
              chunk_id: excerpt.chunkId,
              text: excerpt.text,
            })),
            prompt_versions: [
              GAP_TIER2_PROMPT_VERSION,
              GAP_TIER3_PROMPT_VERSION,
            ],
          }),
        ],
      );
      await client.query(
        `delete from unmatched_chunks where chunk_id = any($1)`,
        [chunkIds],
      );
      result.proposalsCreated++;
    });
  }

  private async attachToOpenProposal(
    client: PoolClient,
    tenantId: string,
    chunks: ClusterChunk[],
  ): Promise<number | null> {
    const chunkIds = chunks.map((c) => c.chunkId);
    const open = await client.query<{ id: string }>(
      `select distinct dp.id from drift_proposals dp
       join drift_evidence de on de.proposal_id = dp.id
       where dp.kind = 'gap' and dp.status in ('pending', 'queued')
         and de.chunk_id = any($1)
       limit 1`,
      [chunkIds],
    );
    if (!open.rows[0]) {
      return null;
    }
    return this.attachEvidence(client, tenantId, open.rows[0].id, chunks);
  }

  private async attachEvidence(
    client: PoolClient,
    tenantId: string,
    proposalId: string,
    chunks: ClusterChunk[],
  ): Promise<number> {
    let attached = 0;
    for (const chunk of chunks) {
      const inserted = await client.query(
        `insert into drift_evidence (tenant_id, proposal_id, event_id, chunk_id)
         select $1, $2, $3, $4
         where not exists (
           select 1 from drift_evidence
           where proposal_id = $2 and chunk_id = $4
         )`,
        [tenantId, proposalId, chunk.eventId, chunk.chunkId],
      );
      attached += inserted.rowCount ?? 0;
    }
    await client.query(
      `delete from unmatched_chunks where chunk_id = any($1)`,
      [chunks.map((c) => c.chunkId)],
    );
    return attached;
  }

  private async routeTarget(
    client: PoolClient,
    domain: string,
  ): Promise<string | null> {
    const domainOwner = await client.query<{ owner_id: string }>(
      `select ce.owner_id from canon_entries ce
       where ce.domain = $1 and ce.status = 'active'
       group by ce.owner_id
       order by count(*) desc, ce.owner_id
       limit 1`,
      [domain],
    );
    if (domainOwner.rows[0]) {
      return domainOwner.rows[0].owner_id;
    }
    const admin = await client.query<{ id: string }>(
      `select id from people where role = 'admin' order by email limit 1`,
    );
    return admin.rows[0]?.id ?? null;
  }

  private async dropFromBuffer(
    tenantId: string,
    chunkIds: string[],
  ): Promise<void> {
    await this.db.withTenant(tenantId, (client) =>
      client.query(`delete from unmatched_chunks where chunk_id = any($1)`, [
        chunkIds,
      ]),
    );
  }
}
