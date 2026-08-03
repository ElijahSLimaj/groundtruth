import { providerFor, OAuthProvider } from './providers';

function oauth(source: string): OAuthProvider {
  const p = providerFor(source);
  if (!p || p.kind !== 'oauth') {
    throw new Error(`${source} not an oauth provider`);
  }
  return p;
}

describe('provider token extraction (documented shapes)', () => {
  it('slack: bot token is nested and team id captured', () => {
    const result = oauth('slack').extract({
      ok: true,
      access_token: 'xoxb-123',
      token_type: 'bot',
      scope: 'channels:history,channels:read',
      bot_user_id: 'U0',
      team: { id: 'T123', name: 'Acme' },
      authed_user: { id: 'U1' },
    });
    expect(result.accessToken).toBe('xoxb-123');
    expect(result.refreshToken).toBeNull();
    expect(result.metadata.team).toBe('T123');
  });

  it('notion: bearer token, no refresh, workspace captured', () => {
    const result = oauth('notion').extract({
      access_token: 'secret_abc',
      token_type: 'bearer',
      bot_id: 'bot_1',
      workspace_id: 'ws_1',
      workspace_name: 'Acme',
    });
    expect(result.accessToken).toBe('secret_abc');
    expect(result.refreshToken).toBeNull();
    expect(result.expiresInSeconds).toBeNull();
    expect(result.metadata.workspace_id).toBe('ws_1');
  });

  it('google: refresh token and expiry parsed', () => {
    const result = oauth('gmail').extract({
      access_token: 'ya29.a',
      expires_in: 3599,
      refresh_token: '1//refresh',
      scope: 'https://www.googleapis.com/auth/gmail.readonly',
      token_type: 'Bearer',
    });
    expect(result.refreshToken).toBe('1//refresh');
    expect(result.expiresInSeconds).toBe(3599);
  });

  it('hubspot: hub_id captured for later API scoping', () => {
    const result = oauth('hubspot').extract({
      token_type: 'bearer',
      refresh_token: 'r',
      access_token: 'a',
      expires_in: 1800,
      hub_id: 42,
    });
    expect(result.metadata.hub_id).toBe(42);
    expect(result.expiresInSeconds).toBe(1800);
  });

  it('salesforce: instance_url captured (all later calls target it)', () => {
    const result = oauth('salesforce').extract({
      access_token: '00Dx',
      instance_url: 'https://acme.my.salesforce.com',
      refresh_token: 'r',
      token_type: 'Bearer',
      issued_at: '1',
    });
    expect(result.metadata.instance_url).toBe('https://acme.my.salesforce.com');
  });

  it('fathom and odoo are api-key, not oauth', () => {
    expect(providerFor('fathom')?.kind).toBe('apikey');
    expect(providerFor('odoo')?.kind).toBe('apikey');
  });
});
