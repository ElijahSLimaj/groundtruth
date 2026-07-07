import {
  IsArray,
  IsBoolean,
  IsInt,
  IsNotEmpty,
  IsObject,
  IsOptional,
  IsString,
  IsUUID,
  Max,
  Min,
} from 'class-validator';

export class QueryRequestDto {
  @IsString()
  @IsNotEmpty()
  question!: string;

  @IsOptional()
  @IsArray()
  @IsString({ each: true })
  domains?: string[];

  @IsOptional()
  @IsBoolean()
  include_stream?: boolean;

  @IsOptional()
  @IsInt()
  @Min(1)
  @Max(20)
  max_citations?: number;
}

export class ProposeUpdateDto {
  @IsOptional()
  @IsUUID()
  entry_id?: string;

  @IsOptional()
  @IsString()
  @IsNotEmpty()
  domain?: string;

  @IsString()
  @IsNotEmpty()
  statement!: string;

  @IsOptional()
  @IsObject()
  attributes?: Record<string, unknown>;
}

export interface CanonCitation {
  type: 'canon';
  entry_id: string;
  version: number;
  verified_at: string | null;
  approver: string | null;
  statement: string;
}

export interface StreamCitation {
  type: 'stream';
  event_id: string;
  chunk_id: string;
  occurred_at: string | null;
  excerpt: string;
}

export type Citation = CanonCitation | StreamCitation;

export interface ConflictSummary {
  entry_id: string | null;
  description: string;
  proposal_id: string;
}

export interface QueryResponse {
  answer: string;
  trust: string;
  citations: Citation[];
  conflicts: ConflictSummary[];
  freshness: {
    oldest_citation: string | null;
    decayed_entries_used: number;
  };
  retrieval_degraded?: boolean;
}
