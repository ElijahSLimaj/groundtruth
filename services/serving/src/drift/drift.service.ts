import { Inject, Injectable, Logger } from '@nestjs/common';
import { PoolClient } from 'pg';

import { DatabaseService } from '../database/database.service';
import { SERVING_CONFIG } from '../config';
import type { ServingConfig } from '../config';
import { LLM_CLIENT } from './llm';
import type { LlmClient } from './llm';
import {
  TIER2_PROMPT_VERSION,
  TIER2_SYSTEM,
  TIER3_PROMPT_VERSION,
  TIER3_SYSTEM,
  tier2UserPrompt,
  tier3UserPrompt,
} from './prompts';
import {
  calibrateConfidence,
  decodeTier3,
  resolveTuning,
  Tier2Result,
  Tier3Result,
  Tier3Wire,
  TuningParams,
} from './schemas';

export interface DriftRunResult {
  disabled: boolean;
  chunksScanned: number;
  tier1Passed: number;
  confirms: number;
  unrelated: number;
  tier3Drafted: number;
  invalidDrafts: number;
  proposalsCreated: number;
  evidenceAttached: number;
  suppressedByCooldown: number;
  queuedByBudget: number;
}

interface Tier1Candidate {
  chunkId: string;
  anchorEventId: string;
  content: string;
  sourceType: string;
  windowKey: string;
  createdAt: Date;
  authorOk: boolean;
  authorKey: string;
  entryId: string | null;
  entryTier: string | null;
  entryDomain: string | null;
  ownerId: string | null;
  statement: string | null;
  attributes: Record<string, unknown> | null;
  similarity: number | null;
}

const emptyResult = (disabled: boolean): DriftRunResult => ({
  disabled,
  chunksScanned: 0,
  tier1Passed: 0,
  confirms: 0,
  unrelated: 0,
  tier3Drafted: 0,
  invalidDrafts: 0,
  proposalsCreated: 0,
  evidenceAttached: 0,
  suppressedByCooldown: 0,
  queuedByBudget: 0,
});

@Injectable()
export class DriftService {
  private readonly logger = new Logger(DriftService.name);

  constructor(
    private readonly db: DatabaseService,
    @Inject(LLM_CLIENT) private readonly llm: LlmClient,
    @Inject(SERVING_CONFIG) private readonly config: ServingConfig,
  ) {}

  async runOnce(tenantId: string): Promise<DriftRunResult> {
    if (!this.llm.enabled) {
      this.logger.warn('drift run skipped, no llm configured');
      return emptyResult(true);
    }
    const result = emptyResult(false);

    const { tuning, candidates } = await this.db.withTenant(
      tenantId,
      async (client) => ({
        tuning: await this.loadTuning(client),
        candidates: await this.scanCandidates(client),
      }),
    );
    result.chunksScanned = candidates.length;
    if (candidates.length === 0) {
      return result;
    }

    const unmatched: Tier1Candidate[] = [];
    for (const candidate of candidates) {
      if (!this.passesTier1(candidate, tuning)) {
        if (candidate.authorOk) {
          unmatched.push(candidate);
        }
        continue;
      }
      result.tier1Passed++;
      await this.classify(tenantId, candidate, tuning, result);
    }
    await this.bufferUnmatched(tenantId, unmatched);

    const last = candidates[candidates.length - 1];
    await this.db.withTenant(tenantId, (client) =>
      client.query(
        `insert into drift_state (tenant_id, last_chunk_created_at, last_chunk_id)
         select $1, c.created_at, c.id from event_chunks c where c.id = $2
         on conflict (tenant_id) do update
         set last_chunk_created_at = excluded.last_chunk_created_at,
             last_chunk_id = excluded.last_chunk_id,
             updated_at = now()`,
        [tenantId, last.chunkId],
      ),
    );
    return result;
  }

  private async loadTuning(client: PoolClient): Promise<TuningParams> {
    const row = await client.query<{ params: unknown }>(
      `select params from drift_tuning where tenant_id = public.app_tenant_id()`,
    );
    return resolveTuning(row.rows[0]?.params);
  }

  private async scanCandidates(client: PoolClient): Promise<Tier1Candidate[]> {
    interface ScanRow {
      chunk_id: string;
      anchor_event_id: string;
      content: string;
      source_type: string;
      window_key: string;
      created_at: Date;
      author_ok: boolean;
      author_key: string;
      entry_id: string | null;
      entry_tier: string | null;
      entry_domain: string | null;
      owner_id: string | null;
      statement: string | null;
      attributes: Record<string, unknown> | null;
      similarity: string | null;
    }
    const rows = await client.query<ScanRow>(
      `
      with wm as (
        select coalesce((select ds.last_chunk_created_at from drift_state ds), 'epoch'::timestamptz) as t,
               coalesce((select ds.last_chunk_id from drift_state ds), '00000000-0000-0000-0000-000000000000'::uuid) as i
      ),
      new_chunks as (
        select c.id, c.event_id, c.content, c.source_type, c.window_key,
               c.created_at, c.embedding, c.embedding_model,
               exists (
                 select 1 from events e
                 where e.id = any(c.member_event_ids) and e.author_source_ref is not null
               ) as author_ok,
               coalesce(
                 (select e.author_source_ref from events e where e.id = c.event_id),
                 'unknown'
               ) as author_key
        from event_chunks c, wm
        where not c.tombstoned and (c.created_at, c.id) > (wm.t, wm.i)
        order by c.created_at, c.id
        limit $1
      )
      select nc.id as chunk_id, nc.event_id as anchor_event_id, nc.content,
             nc.source_type, nc.window_key, nc.created_at, nc.author_ok, nc.author_key,
             cand.entry_id, cand.tier as entry_tier, cand.domain as entry_domain,
             cand.owner_id, cand.statement, cand.attributes, cand.similarity
      from new_chunks nc
      left join lateral (
        select ce.id as entry_id, ce.tier, ce.domain, ce.owner_id,
               cv.statement, cv.attributes,
               1 - (cse.embedding <=> nc.embedding) as similarity
        from canon_statement_embeddings cse
        join canon_versions cv on cv.id = cse.version_id
        join canon_entries ce on ce.current_version_id = cv.id and ce.status = 'active'
        where cse.embedding_model = nc.embedding_model
        order by cse.embedding <=> nc.embedding
        limit 1
      ) cand on true
      order by nc.created_at, nc.id
      `,
      [200],
    );

    return rows.rows.map((row) => ({
      chunkId: row.chunk_id,
      anchorEventId: row.anchor_event_id,
      content: row.content,
      sourceType: row.source_type,
      windowKey: row.window_key,
      createdAt: row.created_at,
      authorOk: row.author_ok,
      authorKey: row.author_key,
      entryId: row.entry_id,
      entryTier: row.entry_tier,
      entryDomain: row.entry_domain,
      ownerId: row.owner_id,
      statement: row.statement,
      attributes: row.attributes,
      similarity: row.similarity === null ? null : Number(row.similarity),
    }));
  }

  private async bufferUnmatched(
    tenantId: string,
    unmatched: Tier1Candidate[],
  ): Promise<void> {
    if (unmatched.length === 0) {
      return;
    }
    await this.db.withTenant(tenantId, async (client) => {
      for (const candidate of unmatched) {
        await client.query(
          `insert into unmatched_chunks (tenant_id, chunk_id, event_id, author_key)
           values ($1, $2, $3, $4)
           on conflict (chunk_id) do nothing`,
          [
            tenantId,
            candidate.chunkId,
            candidate.anchorEventId,
            candidate.authorKey,
          ],
        );
      }
    });
  }

  private passesTier1(
    candidate: Tier1Candidate,
    tuning: TuningParams,
  ): boolean {
    if (!candidate.authorOk || candidate.entryId === null) {
      return false;
    }
    const discount = tuning.tier1_source_discounts[candidate.sourceType] ?? 0;
    const penalty = candidate.entryDomain
      ? (tuning.tier1_domain_penalties[candidate.entryDomain] ?? 0)
      : 0;
    return (
      (candidate.similarity ?? 0) >= tuning.tier1_threshold - discount + penalty
    );
  }

  private async classify(
    tenantId: string,
    candidate: Tier1Candidate,
    tuning: TuningParams,
    result: DriftRunResult,
  ): Promise<void> {
    const tier2 = await this.llm.completeJson(
      {
        model: this.config.driftTier2Model,
        system: TIER2_SYSTEM,
        user: tier2UserPrompt({
          statement: candidate.statement as string,
          attributes: candidate.attributes ?? {},
          chunkContent: candidate.content,
          sourceType: candidate.sourceType,
        }),
        maxTokens: 256,
        promptVersion: TIER2_PROMPT_VERSION,
      },
      Tier2Result,
    );

    if (tier2.relation === 'confirms') {
      result.confirms++;
      await this.db.withTenant(tenantId, (client) =>
        client.query(
          `update canon_entries set last_referenced_at = now() where id = $1`,
          [candidate.entryId],
        ),
      );
      return;
    }
    if (
      tier2.relation === 'unrelated' ||
      tier2.confidence < tuning.tier2_gate
    ) {
      result.unrelated++;
      return;
    }

    const pre = await this.precheck(tenantId, candidate, tier2, tuning, result);
    if (pre.handled) {
      return;
    }

    const draft = await this.draft(tenantId, candidate, pre.corroborating);
    result.tier3Drafted++;
    if (!draft) {
      result.invalidDrafts++;
      return;
    }
    await this.applyHygiene(
      tenantId,
      candidate,
      tier2,
      draft,
      pre.corroborating,
      pre.recurring,
      tuning,
      result,
    );
  }

  private async precheck(
    tenantId: string,
    candidate: Tier1Candidate,
    tier2: Tier2Result,
    tuning: TuningParams,
    result: DriftRunResult,
  ): Promise<{
    handled: boolean;
    recurring: boolean;
    corroborating: { chunkId: string; eventId: string; content: string }[];
  }> {
    return this.db.withTenant(tenantId, async (client) => {
      const open = await client.query<{ id: string }>(
        `select id from drift_proposals
         where entry_id = $1
           and coalesce(conflicting_field, '') = coalesce($2, '')
           and status in ('pending', 'queued')
         limit 1`,
        [candidate.entryId, tier2.conflicting_field],
      );
      if (open.rows[0]) {
        const attached = await this.attachEvidence(
          client,
          tenantId,
          open.rows[0].id,
          [{ chunkId: candidate.chunkId, eventId: candidate.anchorEventId }],
        );
        result.evidenceAttached += attached;
        return { handled: true, recurring: false, corroborating: [] };
      }

      const corroboratingRows = await client.query<{
        id: string;
        event_id: string;
        content: string;
      }>(
        `select c.id, c.event_id, c.content from event_chunks c
         where c.id <> $1 and not c.tombstoned and c.window_key <> $2
         order by c.embedding <=> (select embedding from event_chunks where id = $1 limit 1)
         limit 5`,
        [candidate.chunkId, candidate.windowKey],
      );
      const corroborating = corroboratingRows.rows.map((c) => ({
        chunkId: c.id,
        eventId: c.event_id,
        content: c.content,
      }));

      const rejected = await client.query<{ id: string }>(
        `select id from drift_proposals
         where entry_id = $1
           and coalesce(conflicting_field, '') = coalesce($2, '')
           and status = 'resolved' and resolution <> 'approved'
           and resolved_at > now() - make_interval(days => $3)
         order by resolved_at desc limit 1`,
        [candidate.entryId, tier2.conflicting_field, tuning.cooldown_days],
      );
      if (!rejected.rows[0]) {
        return { handled: false, recurring: false, corroborating };
      }
      const oldEvidence = await client.query<{ n: string }>(
        `select count(*) as n from drift_evidence where proposal_id = $1`,
        [rejected.rows[0].id],
      );
      const oldCount = Math.max(Number(oldEvidence.rows[0].n), 1);
      if (1 + corroborating.length < 2 * oldCount) {
        result.suppressedByCooldown++;
        return { handled: true, recurring: false, corroborating };
      }
      return { handled: false, recurring: true, corroborating };
    });
  }

  private async draft(
    tenantId: string,
    candidate: Tier1Candidate,
    corroborating: { chunkId: string; eventId: string; content: string }[],
  ): Promise<Tier3Result | null> {
    const context = await this.db.withTenant(tenantId, async (client) => {
      const history = await client.query<{
        version_number: number;
        statement: string;
      }>(
        `select cv.version_number, cv.statement from canon_versions cv
         where cv.entry_id = $1 order by cv.version_number desc limit 3`,
        [candidate.entryId],
      );
      const thread = await client.query<{ content: string }>(
        `select c.content from event_chunks c
         where c.window_key = $1 and c.id <> $2 and not c.tombstoned
         order by c.event_occurred_at limit 5`,
        [candidate.windowKey, candidate.chunkId],
      );
      return { history: history.rows, thread: thread.rows };
    });

    const wire = await this.llm.completeJson(
      {
        model: this.config.driftTier3Model,
        system: TIER3_SYSTEM,
        user: tier3UserPrompt({
          statement: candidate.statement as string,
          attributes: candidate.attributes ?? {},
          versionHistory: context.history.map((h) => ({
            version: h.version_number,
            statement: h.statement,
          })),
          triggerContent: candidate.content,
          threadContext: context.thread.map((t) => t.content),
          corroborating: corroborating.map((c) => c.content),
        }),
        maxTokens: 2048,
        promptVersion: TIER3_PROMPT_VERSION,
      },
      Tier3Wire,
    );
    const decoded = decodeTier3(wire);
    if (!decoded) {
      this.logger.warn(
        JSON.stringify({
          event: 'drift_invalid_draft',
          chunk_id: candidate.chunkId,
          entry_id: candidate.entryId,
        }),
      );
      return null;
    }
    return decoded;
  }

  private async applyHygiene(
    tenantId: string,
    candidate: Tier1Candidate,
    tier2: Tier2Result,
    draft: Tier3Result,
    corroborating: { chunkId: string; eventId: string }[],
    recurring: boolean,
    tuning: TuningParams,
    result: DriftRunResult,
  ): Promise<void> {
    const evidence = [
      { chunkId: candidate.chunkId, eventId: candidate.anchorEventId },
      ...corroborating,
    ];
    const confidence = calibrateConfidence(
      draft.confidence,
      tuning.tier3_calibration,
    );

    await this.db.withTenant(tenantId, async (client) => {
      const budgetUsed = await client.query<{ n: string }>(
        `select count(*) as n from drift_proposals
         where routed_to = $1 and status = 'pending'
           and created_at > now() - interval '7 days'`,
        [candidate.ownerId],
      );
      const status =
        Number(budgetUsed.rows[0].n) >= tuning.owner_weekly_budget
          ? 'queued'
          : 'pending';
      if (status === 'queued') {
        result.queuedByBudget++;
      }

      const strategic = candidate.entryTier === 'bedrock';
      let escalatedTo: string | null = null;
      if (strategic) {
        const admin = await client.query<{ id: string }>(
          `select id from people where role = 'admin' order by email limit 1`,
        );
        escalatedTo = admin.rows[0]?.id ?? null;
      }

      const kind = tier2.relation === 'extends' ? 'extension' : 'contradiction';
      const inserted = await client.query<{ id: string }>(
        `insert into drift_proposals
           (tenant_id, entry_id, kind, drafted_statement, drafted_attributes,
            confidence, routed_to, domain, origin, conflicting_field,
            strategic, escalated_to, recurring_after_rejection, status)
         values ($1, $2, $3, $4, $5, $6, $7, $8, 'drift_engine', $9, $10, $11, $12, $13)
         returning id`,
        [
          tenantId,
          candidate.entryId,
          kind,
          draft.draftedStatement,
          JSON.stringify(draft.draftedAttributes),
          confidence,
          candidate.ownerId,
          candidate.entryDomain,
          tier2.conflicting_field,
          strategic,
          escalatedTo,
          recurring,
          status,
        ],
      );
      const proposalId = inserted.rows[0].id;
      await this.attachEvidence(client, tenantId, proposalId, evidence);
      await client.query(
        `insert into audit_log (tenant_id, actor_id, action, subject_type, subject_id, detail)
         values ($1, null, 'drift.proposal.created', 'drift_proposal', $2, $3)`,
        [
          tenantId,
          proposalId,
          JSON.stringify({
            kind,
            entry_id: candidate.entryId,
            confidence,
            raw_confidence: draft.confidence,
            description: draft.contradictionDescription,
            excerpts: draft.supportingExcerpts,
            prompt_versions: [TIER2_PROMPT_VERSION, TIER3_PROMPT_VERSION],
            strategic,
            recurring_after_rejection: recurring,
          }),
        ],
      );
      result.proposalsCreated++;
    });
  }

  private async attachEvidence(
    client: PoolClient,
    tenantId: string,
    proposalId: string,
    evidence: { chunkId: string; eventId: string }[],
  ): Promise<number> {
    let attached = 0;
    for (const item of evidence) {
      const inserted = await client.query(
        `insert into drift_evidence (tenant_id, proposal_id, event_id, chunk_id)
         select $1, $2, $3, $4
         where not exists (
           select 1 from drift_evidence
           where proposal_id = $2 and chunk_id = $4
         )`,
        [tenantId, proposalId, item.eventId, item.chunkId],
      );
      attached += inserted.rowCount ?? 0;
    }
    return attached;
  }
}
