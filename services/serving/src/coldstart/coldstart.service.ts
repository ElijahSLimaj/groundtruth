import { Inject, Injectable, Logger } from '@nestjs/common';
import { PoolClient } from 'pg';

import { DatabaseService } from '../database/database.service';
import { SERVING_CONFIG } from '../config';
import type { ServingConfig } from '../config';
import { LLM_CLIENT } from '../drift/llm';
import type { LlmClient } from '../drift/llm';
import {
  COLDSTART_PROMPT_VERSION,
  COLDSTART_SYSTEM,
  coldStartUserPrompt,
  ORG_INFERENCE_PROMPT_VERSION,
  ORG_INFERENCE_SYSTEM,
  orgInferenceUserPrompt,
} from './prompts';
import type { OrgAuthorStat, OrgChannelStat } from './prompts';
import { ColdStartWire, wordOverlap } from './schemas';

const CANDIDATE_PATTERNS =
  'decided | decision | agreed | pricing | price | discount | policy | quote | effective | approved | process | sla';
const CHUNKS_PER_CALL = 8;
const MAX_CALLS_PER_RUN = 5;
const DUPLICATE_OVERLAP = 0.6;

export interface ColdStartRunResult {
  disabled: boolean;
  chunksScanned: number;
  candidates: number;
  llmCalls: number;
  entriesDrafted: number;
  skippedDuplicates: number;
  budgetBlocked: number;
}

export interface OrgInferenceResult {
  disabled: boolean;
  alreadyInferred: boolean;
  authorsAnalyzed: number;
  entriesDrafted: number;
  skippedDuplicates: number;
  budgetBlocked: number;
}

type DraftCounters = Pick<
  ColdStartRunResult,
  'entriesDrafted' | 'skippedDuplicates' | 'budgetBlocked'
>;

export interface ReviewQueueItem {
  proposal_id: string;
  entry_id: string;
  version_id: string;
  domain: string;
  tier: string;
  statement: string;
  confidence: number;
}

interface CandidateChunk {
  chunkId: string;
  content: string;
  memberEventIds: string[];
  decisionLike: boolean;
}

@Injectable()
export class ColdStartService {
  private readonly logger = new Logger(ColdStartService.name);

  constructor(
    private readonly db: DatabaseService,
    @Inject(LLM_CLIENT) private readonly llm: LlmClient,
    @Inject(SERVING_CONFIG) private readonly config: ServingConfig,
  ) {}

  async runOnce(tenantId: string): Promise<ColdStartRunResult> {
    const result: ColdStartRunResult = {
      disabled: false,
      chunksScanned: 0,
      candidates: 0,
      llmCalls: 0,
      entriesDrafted: 0,
      skippedDuplicates: 0,
      budgetBlocked: 0,
    };
    if (!this.llm.enabled) {
      this.logger.warn('cold start run skipped, no llm configured');
      result.disabled = true;
      return result;
    }

    const scanned = await this.db.withTenant(tenantId, (client) =>
      this.scanChunks(client),
    );
    result.chunksScanned = scanned.length;
    if (scanned.length === 0) {
      return result;
    }
    const candidates = scanned.filter((c) => c.decisionLike);
    result.candidates = candidates.length;

    const reviewers = await this.loadReviewers(tenantId);
    if (!reviewers.ownerId) {
      this.logger.warn('cold start run skipped, no admin or owner to route to');
      return result;
    }

    for (
      let start = 0;
      start < candidates.length && result.llmCalls < MAX_CALLS_PER_RUN;
      start += CHUNKS_PER_CALL
    ) {
      const batch = candidates.slice(start, start + CHUNKS_PER_CALL);
      const wire = await this.llm.completeJson(
        {
          model: this.config.driftTier3Model,
          system: COLDSTART_SYSTEM,
          user: coldStartUserPrompt(batch.map((c) => c.content)),
          maxTokens: 4096,
          promptVersion: COLDSTART_PROMPT_VERSION,
        },
        ColdStartWire,
      );
      result.llmCalls++;
      await this.persistEntries(
        tenantId,
        wire,
        batch,
        reviewers,
        result,
        COLDSTART_PROMPT_VERSION,
      );
    }

    const last = scanned[scanned.length - 1];
    await this.db.withTenant(tenantId, (client) =>
      client.query(
        `insert into cold_start_state (tenant_id, last_chunk_created_at, last_chunk_id, llm_calls)
         select $1, c.created_at, c.id, $3 from event_chunks c where c.id = $2
         on conflict (tenant_id) do update
         set last_chunk_created_at = excluded.last_chunk_created_at,
             last_chunk_id = excluded.last_chunk_id,
             llm_calls = cold_start_state.llm_calls + $3,
             updated_at = now()`,
        [tenantId, last.chunkId, result.llmCalls],
      ),
    );
    return result;
  }

  async inferOrg(tenantId: string): Promise<OrgInferenceResult> {
    const result: OrgInferenceResult = {
      disabled: false,
      alreadyInferred: false,
      authorsAnalyzed: 0,
      entriesDrafted: 0,
      skippedDuplicates: 0,
      budgetBlocked: 0,
    };
    if (!this.llm.enabled) {
      result.disabled = true;
      return result;
    }

    const alreadyInferred = await this.db.withTenant(
      tenantId,
      async (client) => {
        const state = await client.query<{ org_inferred_at: Date | null }>(
          `select org_inferred_at from cold_start_state where tenant_id = $1`,
          [tenantId],
        );
        return state.rows[0]?.org_inferred_at != null;
      },
    );
    if (alreadyInferred) {
      result.alreadyInferred = true;
      return result;
    }

    const stats = await this.db.withTenant(tenantId, (client) =>
      this.communicationStats(client),
    );
    result.authorsAnalyzed = stats.authors.length;
    if (stats.authors.length === 0) {
      return result;
    }

    const reviewers = await this.loadReviewers(tenantId);
    if (!reviewers.ownerId) {
      this.logger.warn('org inference skipped, no admin or owner to route to');
      return result;
    }

    const wire = await this.llm.completeJson(
      {
        model: this.config.driftTier3Model,
        system: ORG_INFERENCE_SYSTEM,
        user: orgInferenceUserPrompt(stats),
        maxTokens: 4096,
        promptVersion: ORG_INFERENCE_PROMPT_VERSION,
      },
      ColdStartWire,
    );
    await this.persistEntries(
      tenantId,
      wire,
      [],
      reviewers,
      result,
      ORG_INFERENCE_PROMPT_VERSION,
    );

    await this.db.withTenant(tenantId, (client) =>
      client.query(
        `insert into cold_start_state (tenant_id, org_inferred_at)
         values ($1, now())
         on conflict (tenant_id) do update
         set org_inferred_at = now(), updated_at = now()`,
        [tenantId],
      ),
    );
    return result;
  }

  private async loadReviewers(
    tenantId: string,
  ): Promise<{ adminId?: string; ownerId?: string }> {
    return this.db.withTenant(tenantId, async (client) => {
      const admin = await client.query<{ id: string }>(
        `select id from people where role = 'admin' order by email limit 1`,
      );
      const owner = await client.query<{ id: string }>(
        `select id from people where role in ('admin', 'owner') order by role, email limit 1`,
      );
      return { adminId: admin.rows[0]?.id, ownerId: owner.rows[0]?.id };
    });
  }

  private async communicationStats(client: PoolClient): Promise<{
    authors: OrgAuthorStat[];
    channels: OrgChannelStat[];
  }> {
    interface AuthorRow {
      author: string;
      messages: number;
      channels: string[];
      threads_started: number;
    }
    const authorRows = await client.query<AuthorRow>(
      `
      with recent as (
        select coalesce(p.display_name, e.author_source_ref) as author,
               split_part(e.thread_key, ':', 1) as channel,
               e.thread_key, e.occurred_at
        from events e
        left join people p on p.id = e.author_id
        where e.author_source_ref is not null and not e.tombstoned
          and e.thread_key is not null
          and e.occurred_at > now() - interval '90 days'
      ),
      starters as (
        select distinct on (thread_key) thread_key, author
        from recent order by thread_key, occurred_at
      )
      select r.author,
             count(*)::int as messages,
             (array_agg(distinct r.channel))[1:5] as channels,
             (select count(*)::int from starters s where s.author = r.author) as threads_started
      from recent r
      group by r.author
      order by messages desc
      limit 20
      `,
    );
    interface ChannelRow {
      channel: string;
      messages: number;
      top_authors: string[];
    }
    const channelRows = await client.query<ChannelRow>(
      `
      with recent as (
        select coalesce(p.display_name, e.author_source_ref) as author,
               split_part(e.thread_key, ':', 1) as channel
        from events e
        left join people p on p.id = e.author_id
        where e.author_source_ref is not null and not e.tombstoned
          and e.thread_key is not null
          and e.occurred_at > now() - interval '90 days'
      ),
      by_channel_author as (
        select channel, author, count(*) as n
        from recent group by channel, author
      )
      select channel,
             sum(n)::int as messages,
             (array_agg(author order by n desc))[1:3] as top_authors
      from by_channel_author
      group by channel
      order by messages desc
      limit 15
      `,
    );
    return {
      authors: authorRows.rows.map((r) => ({
        author: r.author,
        messages: r.messages,
        channels: r.channels,
        threadsStarted: r.threads_started,
      })),
      channels: channelRows.rows.map((r) => ({
        channel: r.channel,
        messages: r.messages,
        topAuthors: r.top_authors,
      })),
    };
  }

  async reviewQueue(tenantId: string): Promise<ReviewQueueItem[]> {
    return this.db.withTenant(tenantId, async (client) => {
      const rows = await client.query<ReviewQueueItem>(
        `select dp.id as proposal_id, dp.entry_id, dp.pending_version_id as version_id,
                ce.domain, ce.tier, cv.statement, dp.confidence::float as confidence
         from drift_proposals dp
         join canon_entries ce on ce.id = dp.entry_id
         join canon_versions cv on cv.id = dp.pending_version_id
         where dp.origin = 'cold_start' and dp.status = 'pending'
         order by (ce.tier = 'bedrock') desc, (ce.domain = 'pricing') desc, dp.confidence desc`,
      );
      return rows.rows;
    });
  }

  private async scanChunks(client: PoolClient): Promise<CandidateChunk[]> {
    interface Row {
      id: string;
      content: string;
      member_event_ids: string[];
      decision_like: boolean;
    }
    const rows = await client.query<Row>(
      `
      with wm as (
        select coalesce((select cs.last_chunk_created_at from cold_start_state cs), 'epoch'::timestamptz) as t,
               coalesce((select cs.last_chunk_id from cold_start_state cs), '00000000-0000-0000-0000-000000000000'::uuid) as i
      )
      select c.id, c.content, c.member_event_ids,
             to_tsvector('english', c.content) @@ to_tsquery('english', $1) as decision_like
      from event_chunks c, wm
      where not c.tombstoned and (c.created_at, c.id) > (wm.t, wm.i)
      order by c.created_at, c.id
      limit $2
      `,
      [CANDIDATE_PATTERNS, 500],
    );
    return rows.rows.map((row) => ({
      chunkId: row.id,
      content: row.content,
      memberEventIds: row.member_event_ids,
      decisionLike: row.decision_like,
    }));
  }

  private async persistEntries(
    tenantId: string,
    wire: ColdStartWire,
    batch: CandidateChunk[],
    reviewers: { adminId?: string; ownerId?: string },
    result: DraftCounters,
    promptVersion: string,
  ): Promise<void> {
    const seenTopics = new Set<string>();

    for (const entry of wire.entries) {
      if (seenTopics.has(`${entry.domain}:${entry.topic}`)) {
        result.skippedDuplicates++;
        continue;
      }
      seenTopics.add(`${entry.domain}:${entry.topic}`);

      let attributes: Record<string, unknown>;
      try {
        const parsed: unknown = JSON.parse(entry.attributes_json);
        if (
          typeof parsed !== 'object' ||
          parsed === null ||
          Array.isArray(parsed)
        ) {
          throw new Error('attributes must be an object');
        }
        attributes = parsed as Record<string, unknown>;
      } catch {
        attributes = {};
      }

      const sourceEventIds = [
        ...new Set(
          entry.source_chunk_indexes
            .filter((index) => index >= 0 && index < batch.length)
            .flatMap((index) => batch[index].memberEventIds),
        ),
      ];

      const created = await this.db.withTenant(tenantId, async (client) => {
        const existing = await client.query<{ statement: string }>(
          `select distinct on (ce.id) cv.statement
           from canon_entries ce
           join canon_versions cv on cv.entry_id = ce.id
           where ce.domain = $1 and ce.status <> 'archived'
           order by ce.id, cv.version_number desc`,
          [entry.domain],
        );
        const duplicate = existing.rows.some(
          (row) =>
            wordOverlap(row.statement, entry.statement) >= DUPLICATE_OVERLAP,
        );
        if (duplicate) {
          return 'duplicate' as const;
        }

        try {
          const draft = await client.query<{
            entry_id: string;
            version_id: string;
          }>(
            `select * from public.canon_create_entry_draft($1, $2, $3, $4, $5, '{"scope": "tenant"}', interval '60 days', $6)`,
            [
              entry.domain,
              entry.tier,
              reviewers.ownerId,
              entry.statement,
              JSON.stringify(attributes),
              sourceEventIds,
            ],
          );
          const { entry_id, version_id } = draft.rows[0];
          await client.query(
            `insert into drift_proposals
               (tenant_id, entry_id, kind, drafted_statement, drafted_attributes,
                confidence, routed_to, domain, origin, pending_version_id, strategic, escalated_to)
             values ($1, $2, 'gap', $3, $4, $5, $6, $7, 'cold_start', $8, $9, $10)`,
            [
              tenantId,
              entry_id,
              entry.statement,
              JSON.stringify(attributes),
              entry.confidence,
              reviewers.ownerId,
              entry.domain,
              version_id,
              entry.tier === 'bedrock',
              entry.tier === 'bedrock' ? (reviewers.adminId ?? null) : null,
            ],
          );
          await client.query(
            `insert into audit_log (tenant_id, actor_id, action, subject_type, subject_id, detail)
             values ($1, null, 'coldstart.entry.drafted', 'canon_entry', $2, $3)`,
            [
              tenantId,
              entry_id,
              JSON.stringify({
                domain: entry.domain,
                tier: entry.tier,
                topic: entry.topic,
                confidence: entry.confidence,
                prompt_version: promptVersion,
              }),
            ],
          );
          return 'created' as const;
        } catch (error) {
          if (String(error).includes('entry budget')) {
            return 'budget' as const;
          }
          throw error;
        }
      });

      if (created === 'created') {
        result.entriesDrafted++;
      } else if (created === 'duplicate') {
        result.skippedDuplicates++;
      } else {
        result.budgetBlocked++;
      }
    }
  }
}
