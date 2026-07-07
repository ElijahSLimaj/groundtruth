import {
  ForbiddenException,
  Injectable,
  NotFoundException,
  UnprocessableEntityException,
} from '@nestjs/common';
import { PoolClient } from 'pg';

import { DatabaseService } from '../database/database.service';
import { Principal } from '../auth/principal';
import {
  CanonCitation,
  ConflictSummary,
  ProposeUpdateDto,
  QueryRequestDto,
  QueryResponse,
} from './dto';
import { deriveTrust } from './trust';

const NO_COVERAGE_ANSWER = 'No governed knowledge covers this question.';

interface CitationRow {
  entry_id: string;
  domain: string;
  entry_status: string;
  verified_at: Date | null;
  version_number: number;
  statement: string;
  approver_id: string | null;
}

@Injectable()
export class CanonService {
  constructor(private readonly db: DatabaseService) {}

  async query(
    principal: Principal,
    dto: QueryRequestDto,
  ): Promise<QueryResponse> {
    const domains = this.effectiveDomains(principal, dto.domains);
    const maxCitations = dto.max_citations ?? 5;

    return this.db.withTenant(principal.tenantId, async (client) => {
      const cited = await client.query<CitationRow>(
        `
        select e.id as entry_id, e.domain, e.status as entry_status, e.verified_at,
               v.version_number, v.statement, a.approver_id
        from canon_entries e
        join canon_versions v on v.id = e.current_version_id
        left join lateral (
          select ap.approver_id from approvals ap
          where ap.version_id = v.id and ap.decision = 'approved'
          order by ap.decided_at desc limit 1
        ) a on true
        where e.status in ('active', 'decayed')
          and ($2::text[] is null or e.domain = any($2))
          and (e.visibility->>'scope' = 'tenant'
               or (e.visibility->>'scope' = 'principals' and e.visibility->'principals' ?| $3))
          and to_tsvector('english', v.statement) @@ websearch_to_tsquery('english', $1)
        order by ts_rank_cd(to_tsvector('english', v.statement), websearch_to_tsquery('english', $1)) desc,
                 e.verified_at desc nulls last
        limit $4
        `,
        [dto.question, domains, principal.principals, maxCitations],
      );

      const citations: CanonCitation[] = cited.rows.map((row) => ({
        type: 'canon',
        entry_id: row.entry_id,
        version: row.version_number,
        verified_at: toDateString(row.verified_at),
        approver: row.approver_id ? `person:${row.approver_id}` : null,
        statement: row.statement,
      }));

      const conflicts = await this.openConflicts(
        client,
        cited.rows.map((row) => row.entry_id),
      );

      const trust = deriveTrust(cited.rows.map((row) => row.entry_status));
      const decayedUsed = cited.rows.filter(
        (row) => row.entry_status === 'decayed',
      ).length;
      const verifiedDates = cited.rows
        .map((row) => row.verified_at)
        .filter((d): d is Date => d !== null);

      const response: QueryResponse = {
        answer:
          citations.length > 0 ? citations[0].statement : NO_COVERAGE_ANSWER,
        trust,
        citations,
        conflicts,
        freshness: {
          oldest_citation:
            verifiedDates.length > 0
              ? toDateString(
                  new Date(Math.min(...verifiedDates.map((d) => d.getTime()))),
                )
              : null,
          decayed_entries_used: decayedUsed,
        },
      };

      await this.audit(client, principal, 'serving.query', {
        tool: 'query',
        entry_ids: cited.rows.map((row) => row.entry_id),
        trust,
      });
      return response;
    });
  }

  async getEntry(
    principal: Principal,
    entryId: string,
  ): Promise<Record<string, unknown>> {
    return this.db.withTenant(principal.tenantId, async (client) => {
      const entry = await client.query(
        `
        select e.id, e.domain, e.tier, e.status, e.owner_id, e.verified_at,
               e.verify_interval::text as verify_interval, e.visibility,
               v.id as version_id, v.version_number, v.statement, v.attributes
        from canon_entries e
        left join canon_versions v on v.id = e.current_version_id
        where e.id = $1
          and (e.visibility->>'scope' = 'tenant'
               or (e.visibility->>'scope' = 'principals' and e.visibility->'principals' ?| $2))
        `,
        [entryId, principal.principals],
      );
      const row = entry.rows[0] as
        (Record<string, unknown> & { domain: string }) | undefined;
      if (!row || !this.domainAllowed(principal, row.domain)) {
        throw new NotFoundException('canon entry not found');
      }

      const [versions, provenance, relations] = await Promise.all([
        client.query(
          `select version_number, statement, attributes, status, created_by, created_at
           from canon_versions where entry_id = $1 order by version_number desc`,
          [entryId],
        ),
        client.query(
          `select cp.version_id, cp.event_id
           from canon_provenance cp
           join canon_versions cv on cv.id = cp.version_id
           where cv.entry_id = $1`,
          [entryId],
        ),
        client.query(
          `select from_entry, to_entry, relation
           from canon_relations where from_entry = $1 or to_entry = $1`,
          [entryId],
        ),
      ]);

      await this.audit(client, principal, 'serving.get_entry', {
        tool: 'get_entry',
        entry_ids: [entryId],
      });

      return {
        entry: row,
        versions: versions.rows,
        provenance: provenance.rows,
        relations: relations.rows,
      };
    });
  }

  async listConflicts(
    principal: Principal,
    domain?: string,
  ): Promise<ConflictSummary[]> {
    if (domain && !this.domainAllowed(principal, domain)) {
      throw new ForbiddenException('domain outside key scope');
    }
    const domains = domain
      ? [domain]
      : (this.effectiveDomains(principal, undefined) ?? null);

    return this.db.withTenant(principal.tenantId, async (client) => {
      const result = await client.query<{
        proposal_id: string;
        entry_id: string | null;
        drafted_statement: string;
      }>(
        `
        select dp.id as proposal_id, dp.entry_id, dp.drafted_statement
        from drift_proposals dp
        left join canon_entries e on e.id = dp.entry_id
        where dp.kind = 'contradiction' and dp.status = 'pending'
          and ($1::text[] is null or coalesce(e.domain, dp.domain) = any($1))
          and (e.id is null
               or e.visibility->>'scope' = 'tenant'
               or (e.visibility->>'scope' = 'principals' and e.visibility->'principals' ?| $2))
        order by dp.created_at desc
        `,
        [domains, principal.principals],
      );

      await this.audit(client, principal, 'serving.list_conflicts', {
        tool: 'list_conflicts',
        entry_ids: result.rows.flatMap((r) => (r.entry_id ? [r.entry_id] : [])),
      });

      return result.rows.map((row) => ({
        entry_id: row.entry_id,
        description: row.drafted_statement,
        proposal_id: row.proposal_id,
      }));
    });
  }

  async proposeUpdate(
    principal: Principal,
    dto: ProposeUpdateDto,
  ): Promise<{ proposal_id: string; kind: string; routed_to: string }> {
    if (!dto.entry_id && !dto.domain) {
      throw new UnprocessableEntityException(
        'either entry_id or domain is required',
      );
    }

    return this.db.withTenant(principal.tenantId, async (client) => {
      let kind: string;
      let domain: string;
      let routedTo: string;
      let entryId: string | null = null;

      if (dto.entry_id) {
        const entry = await client.query<{
          id: string;
          domain: string;
          owner_id: string;
        }>(
          `
          select e.id, e.domain, e.owner_id from canon_entries e
          where e.id = $1
            and (e.visibility->>'scope' = 'tenant'
                 or (e.visibility->>'scope' = 'principals' and e.visibility->'principals' ?| $2))
          `,
          [dto.entry_id, principal.principals],
        );
        const row = entry.rows[0];
        if (!row || !this.domainAllowed(principal, row.domain)) {
          throw new NotFoundException('canon entry not found');
        }
        kind = 'contradiction';
        domain = row.domain;
        routedTo = row.owner_id;
        entryId = row.id;
      } else {
        if (!this.domainAllowed(principal, dto.domain as string)) {
          throw new ForbiddenException('domain outside key scope');
        }
        kind = 'gap';
        domain = dto.domain as string;
        const admin = await client.query<{ id: string }>(
          `select id from people where role in ('admin', 'owner') order by role, email limit 1`,
        );
        if (!admin.rows[0]) {
          throw new UnprocessableEntityException(
            'no admin or owner exists to route the proposal to',
          );
        }
        routedTo = admin.rows[0].id;
      }

      const inserted = await client.query<{ id: string }>(
        `
        insert into drift_proposals
          (tenant_id, entry_id, kind, drafted_statement, drafted_attributes,
           confidence, routed_to, domain, origin)
        values ($1, $2, $3, $4, $5, 0.5, $6, $7, 'agent')
        returning id
        `,
        [
          principal.tenantId,
          entryId,
          kind,
          dto.statement,
          JSON.stringify(dto.attributes ?? {}),
          routedTo,
          domain,
        ],
      );

      await this.audit(client, principal, 'serving.propose_update', {
        tool: 'propose_update',
        entry_ids: entryId ? [entryId] : [],
        proposal_id: inserted.rows[0].id,
      });

      return { proposal_id: inserted.rows[0].id, kind, routed_to: routedTo };
    });
  }

  private async openConflicts(
    client: PoolClient,
    entryIds: string[],
  ): Promise<ConflictSummary[]> {
    if (entryIds.length === 0) {
      return [];
    }
    const result = await client.query<{
      proposal_id: string;
      entry_id: string;
      drafted_statement: string;
    }>(
      `select dp.id as proposal_id, dp.entry_id, dp.drafted_statement
       from drift_proposals dp
       where dp.kind = 'contradiction' and dp.status = 'pending' and dp.entry_id = any($1)`,
      [entryIds],
    );
    return result.rows.map((row) => ({
      entry_id: row.entry_id,
      description: row.drafted_statement,
      proposal_id: row.proposal_id,
    }));
  }

  private effectiveDomains(
    principal: Principal,
    requested: string[] | undefined,
  ): string[] | null {
    if (!principal.allowedDomains) {
      return requested ?? null;
    }
    if (!requested || requested.length === 0) {
      return principal.allowedDomains;
    }
    const outside = requested.filter(
      (d) => !principal.allowedDomains?.includes(d),
    );
    if (outside.length > 0) {
      throw new ForbiddenException('domain outside key scope');
    }
    return requested;
  }

  private domainAllowed(principal: Principal, domain: string): boolean {
    return (
      principal.allowedDomains === null ||
      principal.allowedDomains.includes(domain)
    );
  }

  private async audit(
    client: PoolClient,
    principal: Principal,
    action: string,
    detail: Record<string, unknown>,
  ): Promise<void> {
    await client.query(
      `insert into audit_log (tenant_id, actor_id, action, subject_type, detail)
       values ($1, $2, $3, 'serving_call', $4)`,
      [principal.tenantId, principal.personId, action, JSON.stringify(detail)],
    );
  }
}

function toDateString(date: Date | null): string | null {
  return date ? date.toISOString().slice(0, 10) : null;
}
