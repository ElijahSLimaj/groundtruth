export const SERVING_CONFIG = Symbol('SERVING_CONFIG');

export interface ServingConfig {
  databaseUrl: string;
  port: number;
  anthropicApiKey: string | null;
  driftTier2Model: string;
  driftTier3Model: string;
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
  };
}
