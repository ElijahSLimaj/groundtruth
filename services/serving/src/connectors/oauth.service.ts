import { createHash, randomBytes } from 'node:crypto';

import {
  BadRequestException,
  Inject,
  Injectable,
  UnauthorizedException,
} from '@nestjs/common';

import { SERVING_CONFIG } from '../config';
import type { ServingConfig } from '../config';
import { DatabaseService } from '../database/database.service';
import { ConnectResult, writeConnectorSecret } from './connector-writer';
import { OAuthProvider, providerFor, TokenResult } from './providers';
import {
  masterKey,
  openConnectorSecret,
  sealConnectorSecret,
  tenantDataKey,
} from './secret-crypto';

export type FetchLike = typeof fetch;

interface Credentials {
  clientId: string;
  clientSecret: string;
}

@Injectable()
export class OAuthService {
  fetchImpl: FetchLike = fetch;

  constructor(
    private readonly db: DatabaseService,
    @Inject(SERVING_CONFIG) private readonly config: ServingConfig,
  ) {}

  private oauthProvider(source: string): OAuthProvider {
    const provider = providerFor(source);
    if (!provider || provider.kind !== 'oauth') {
      throw new BadRequestException(`${source} is not an OAuth connector`);
    }
    return provider;
  }

  private credentials(provider: OAuthProvider): Credentials {
    const creds = this.config.connectorCredentials[provider.credentialKey];
    if (!creds) {
      throw new BadRequestException(
        `${provider.source} is not configured, set ${provider.credentialKey}_CLIENT_ID and _CLIENT_SECRET`,
      );
    }
    return creds;
  }

  private redirectUri(source: string): string {
    if (!this.config.appUrl) {
      throw new BadRequestException('APP_URL is not configured');
    }
    return `${this.config.appUrl.replace(/\/$/, '')}/api/connectors/${source}/callback`;
  }

  async start(
    tenantId: string,
    personId: string,
    source: string,
  ): Promise<{ authorize_url: string }> {
    const provider = this.oauthProvider(source);
    const creds = this.credentials(provider);
    const state = randomBytes(24).toString('base64url');
    const verifier = provider.usePkce
      ? randomBytes(48).toString('base64url')
      : '';
    const redirectUri = this.redirectUri(source);

    await this.db.withTenant(tenantId, (client) =>
      client.query(
        `insert into oauth_flows (state, tenant_id, person_id, source_type, code_verifier, redirect_uri)
         values ($1, $2, $3, $4, $5, $6)`,
        [state, tenantId, personId, source, verifier, redirectUri],
      ),
    );

    const params = new URLSearchParams({
      client_id: creds.clientId,
      redirect_uri: redirectUri,
      response_type: 'code',
      state,
      ...provider.authParams,
    });
    if (provider.scope) {
      params.set('scope', provider.scope);
    }
    if (provider.usePkce) {
      params.set(
        'code_challenge',
        createHash('sha256').update(verifier).digest('base64url'),
      );
      params.set('code_challenge_method', 'S256');
    }
    return { authorize_url: `${provider.authorizeUrl}?${params.toString()}` };
  }

  async callback(state: string, code: string): Promise<ConnectResult> {
    const flow = await this.db.asApp(async (client) => {
      const rows = await client.query<{
        tenant_id: string;
        person_id: string;
        source_type: string;
        code_verifier: string;
        redirect_uri: string;
      }>(`select * from public.oauth_flow_consume($1)`, [state]);
      return rows.rows[0];
    });
    if (!flow) {
      throw new UnauthorizedException('oauth state is invalid or expired');
    }

    const provider = this.oauthProvider(flow.source_type);
    const token = await this.exchange(provider, code, flow);
    if (!token.accessToken) {
      throw new BadRequestException(
        `${provider.source} returned no access token`,
      );
    }
    if (!this.config.masterKeyHex) {
      throw new BadRequestException('MASTER_KEY is not configured');
    }
    return writeConnectorSecret({
      db: this.db,
      masterKeyHex: this.config.masterKeyHex,
      tenantId: flow.tenant_id,
      source: provider.source,
      secret: {
        access_token: token.accessToken,
        refresh_token: token.refreshToken,
      },
      clearConfig: {
        expires_at: token.expiresInSeconds
          ? new Date(Date.now() + token.expiresInSeconds * 1000).toISOString()
          : null,
        source_scope: token.metadata,
      },
    });
  }

  private async exchange(
    provider: OAuthProvider,
    code: string,
    flow: { code_verifier: string; redirect_uri: string },
  ): Promise<TokenResult> {
    const body = new URLSearchParams({
      grant_type: 'authorization_code',
      code,
      redirect_uri: flow.redirect_uri,
    });
    if (provider.usePkce) {
      body.set('code_verifier', flow.code_verifier);
    }
    return this.postToken(provider, body);
  }

  private async postToken(
    provider: OAuthProvider,
    body: URLSearchParams,
  ): Promise<TokenResult> {
    const creds = this.credentials(provider);
    body.set('client_id', creds.clientId);
    const headers: Record<string, string> = {
      'content-type': 'application/x-www-form-urlencoded',
      accept: 'application/json',
    };
    if (provider.tokenAuth === 'basic') {
      headers.authorization = `Basic ${Buffer.from(
        `${creds.clientId}:${creds.clientSecret}`,
      ).toString('base64')}`;
    } else {
      body.set('client_secret', creds.clientSecret);
    }

    const response = await this.fetchImpl(provider.tokenUrl, {
      method: 'POST',
      headers,
      body: body.toString(),
    });
    const json = (await response.json()) as Record<string, unknown>;
    if (!response.ok || json['ok'] === false || json['error']) {
      throw new BadRequestException(
        `${provider.source} token request failed: ${
          (json['error'] as string) ?? response.status
        }`,
      );
    }
    return provider.extract(json);
  }

  async refreshExpiring(tenantId: string): Promise<{ refreshed: number }> {
    if (!this.config.masterKeyHex) {
      return { refreshed: 0 };
    }
    const master = masterKey(this.config.masterKeyHex);
    return this.db.withTenant(tenantId, async (client) => {
      const due = await client.query<{
        id: string;
        source_type: string;
        config: { secret?: string; source_scope?: unknown };
      }>(
        `select id, source_type, config from connectors
         where status = 'live'
           and config->>'expires_at' is not null
           and (config->>'expires_at')::timestamptz < now() + interval '5 minutes'`,
      );
      if (due.rows.length === 0) {
        return { refreshed: 0 };
      }
      const dataKey = await tenantDataKey(client, master);
      let refreshed = 0;
      for (const row of due.rows) {
        const provider = providerFor(row.source_type);
        if (!provider || provider.kind !== 'oauth' || !row.config.secret) {
          continue;
        }
        const current = openConnectorSecret<{
          access_token: string;
          refresh_token: string | null;
        }>(dataKey, row.id, row.config.secret);
        if (!current.refresh_token) {
          continue;
        }
        const token = await this.postToken(
          provider,
          new URLSearchParams({
            grant_type: 'refresh_token',
            refresh_token: current.refresh_token,
          }),
        );
        const sealed = sealConnectorSecret(dataKey, row.id, {
          access_token: token.accessToken,
          refresh_token: token.refreshToken ?? current.refresh_token,
        });
        await client.query(
          `update connectors
           set config = jsonb_set(
             jsonb_set(config, '{secret}', to_jsonb($2::text)),
             '{expires_at}',
             to_jsonb($3::text)
           )
           where id = $1`,
          [
            row.id,
            sealed,
            token.expiresInSeconds
              ? new Date(
                  Date.now() + token.expiresInSeconds * 1000,
                ).toISOString()
              : null,
          ],
        );
        refreshed++;
      }
      return { refreshed };
    });
  }
}
