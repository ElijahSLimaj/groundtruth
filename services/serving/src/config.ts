export const SERVING_CONFIG = Symbol('SERVING_CONFIG');

export interface ServingConfig {
  databaseUrl: string;
  port: number;
  anthropicApiKey: string | null;
  driftTier2Model: string;
  driftTier3Model: string;
  embeddingModel: string;
  slackBotToken: string | null;
  slackSigningSecret: string | null;
  slackApprovalChannel: string | null;
  slackTenantId: string | null;
  schedulerEnabled: boolean;
  driftIntervalMs: number;
  decayIntervalMs: number;
  mergeIntervalMs: number;
  mergeSimilarityThreshold: number;
  payloadRoot: string | null;
  s3Bucket: string | null;
  s3Endpoint: string | null;
  s3Region: string | null;
}

export function loadConfig(
  env: NodeJS.ProcessEnv = process.env,
): ServingConfig {
  const databaseUrl = env.DATABASE_URL;
  if (!databaseUrl) {
    throw new Error('DATABASE_URL is required');
  }
  const port = Number(env.PORT ?? 3000);
  if (!Number.isInteger(port) || port < 1 || port > 65535) {
    throw new Error(`PORT must be a valid port number, got ${env.PORT}`);
  }
  return {
    databaseUrl,
    port,
    anthropicApiKey: env.ANTHROPIC_API_KEY ?? null,
    driftTier2Model: env.DRIFT_TIER2_MODEL ?? 'claude-haiku-4-5',
    driftTier3Model: env.DRIFT_TIER3_MODEL ?? 'claude-opus-4-8',
    embeddingModel: env.EMBEDDING_MODEL ?? 'fake-embedder-v1',
    slackBotToken: env.SLACK_BOT_TOKEN ?? null,
    slackSigningSecret: env.SLACK_SIGNING_SECRET ?? null,
    slackApprovalChannel: env.SLACK_APPROVAL_CHANNEL ?? null,
    slackTenantId: env.SLACK_TENANT_ID ?? null,
    schedulerEnabled: env.SCHEDULER_ENABLED === '1',
    driftIntervalMs: Number(env.DRIFT_INTERVAL_MS ?? 300_000),
    decayIntervalMs: Number(env.DECAY_INTERVAL_MS ?? 86_400_000),
    mergeIntervalMs: Number(env.MERGE_INTERVAL_MS ?? 604_800_000),
    mergeSimilarityThreshold: Number(env.MERGE_SIMILARITY_THRESHOLD ?? 0.9),
    payloadRoot: env.PAYLOAD_ROOT ?? null,
    s3Bucket: env.S3_BUCKET ?? null,
    s3Endpoint: env.S3_ENDPOINT ?? null,
    s3Region: env.S3_REGION ?? null,
  };
}
