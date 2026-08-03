import { BadRequestException, UnauthorizedException } from '@nestjs/common';
import type { PoolClient } from 'pg';

import type { ServingConfig } from '../config';
import type { DatabaseService } from '../database/database.service';
import { OAuthService } from './oauth.service';

const client = {
  query: () => Promise.resolve({ rows: [] as unknown[], rowCount: 0 }),
} as unknown as PoolClient;

function fakeDb(consumeRows: unknown[] = []): DatabaseService {
  return {
    withTenant: <T>(_t: string, fn: (c: PoolClient) => Promise<T>) =>
      fn(client),
    asApp: <T>(fn: (c: PoolClient) => Promise<T>) =>
      fn({
        query: () =>
          Promise.resolve({ rows: consumeRows, rowCount: consumeRows.length }),
      } as unknown as PoolClient),
  } as unknown as DatabaseService;
}

function config(): ServingConfig {
  return {
    appUrl: 'https://app.groundtruth.test',
    masterKeyHex: '2a'.repeat(32),
    connectorCredentials: {
      GOOGLE: { clientId: 'g-id', clientSecret: 'g-secret' },
      SLACK: { clientId: 's-id', clientSecret: 's-secret' },
    },
  } as unknown as ServingConfig;
}

describe('OAuthService.start', () => {
  it('builds a PKCE authorize url for google with offline access', async () => {
    const svc = new OAuthService(fakeDb(), config());
    const { authorize_url } = await svc.start('t1', 'p1', 'gmail');
    const url = new URL(authorize_url);
    expect(url.origin + url.pathname).toBe(
      'https://accounts.google.com/o/oauth2/v2/auth',
    );
    expect(url.searchParams.get('client_id')).toBe('g-id');
    expect(url.searchParams.get('response_type')).toBe('code');
    expect(url.searchParams.get('redirect_uri')).toBe(
      'https://app.groundtruth.test/api/connectors/gmail/callback',
    );
    expect(url.searchParams.get('access_type')).toBe('offline');
    expect(url.searchParams.get('prompt')).toBe('consent');
    expect(url.searchParams.get('code_challenge_method')).toBe('S256');
    expect(url.searchParams.get('code_challenge')).toBeTruthy();
    expect(url.searchParams.get('state')).toBeTruthy();
  });

  it('omits PKCE for slack (unsupported) and sets bot scope', async () => {
    const svc = new OAuthService(fakeDb(), config());
    const { authorize_url } = await svc.start('t1', 'p1', 'slack');
    const url = new URL(authorize_url);
    expect(url.searchParams.get('code_challenge')).toBeNull();
    expect(url.searchParams.get('scope')).toContain('channels:history');
  });

  it('rejects an unconfigured provider', async () => {
    const svc = new OAuthService(fakeDb(), config());
    await expect(svc.start('t1', 'p1', 'notion')).rejects.toBeInstanceOf(
      BadRequestException,
    );
  });
});

describe('OAuthService.callback', () => {
  it('rejects an invalid or expired state', async () => {
    const svc = new OAuthService(fakeDb([]), config());
    await expect(svc.callback('bad-state', 'code')).rejects.toBeInstanceOf(
      UnauthorizedException,
    );
  });
});

describe('OAuthService.refreshExpiring', () => {
  it('no-ops when nothing is due', async () => {
    const svc = new OAuthService(fakeDb(), {
      ...config(),
      connectorCredentials: {
        GOOGLE: { clientId: 'g', clientSecret: 's' },
      },
    });
    const result = await svc.refreshExpiring('t1');
    expect(result.refreshed).toBe(0);
  });

  it('no-ops without a master key', async () => {
    const svc = new OAuthService(fakeDb(), {
      ...config(),
      masterKeyHex: null,
    });
    expect(await svc.refreshExpiring('t1')).toEqual({ refreshed: 0 });
  });
});
