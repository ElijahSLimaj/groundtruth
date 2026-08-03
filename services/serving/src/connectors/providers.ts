export interface TokenResult {
  accessToken: string;
  refreshToken: string | null;
  expiresInSeconds: number | null;
  metadata: Record<string, unknown>;
}

export interface OAuthProvider {
  source: string;
  kind: 'oauth';
  credentialKey: string;
  authorizeUrl: string;
  tokenUrl: string;
  scope: string;
  usePkce: boolean;
  tokenAuth: 'body' | 'basic';
  authParams: Record<string, string>;
  extract: (json: Record<string, unknown>) => TokenResult;
}

export interface ApiKeyProvider {
  source: string;
  kind: 'apikey';
  label: string;
  needsBaseUrl: boolean;
}

export type Provider = OAuthProvider | ApiKeyProvider;

function str(json: Record<string, unknown>, key: string): string | null {
  const value = json[key];
  return typeof value === 'string' && value.length > 0 ? value : null;
}

function num(json: Record<string, unknown>, key: string): number | null {
  const value = json[key];
  return typeof value === 'number' ? value : null;
}

const standardExtract = (json: Record<string, unknown>): TokenResult => ({
  accessToken: str(json, 'access_token') ?? '',
  refreshToken: str(json, 'refresh_token'),
  expiresInSeconds: num(json, 'expires_in'),
  metadata: {
    scope: str(json, 'scope'),
    instance_url: str(json, 'instance_url'),
    hub_id: json['hub_id'] ?? null,
  },
});

export const OAUTH_PROVIDERS: OAuthProvider[] = [
  {
    source: 'slack',
    kind: 'oauth',
    credentialKey: 'SLACK',
    authorizeUrl: 'https://slack.com/oauth/v2/authorize',
    tokenUrl: 'https://slack.com/api/oauth.v2.access',
    scope:
      'channels:history,channels:read,groups:history,groups:read,users:read',
    usePkce: false,
    tokenAuth: 'body',
    authParams: {},
    extract: (json) => {
      const team = json['team'] as Record<string, unknown> | undefined;
      return {
        accessToken: str(json, 'access_token') ?? '',
        refreshToken: str(json, 'refresh_token'),
        expiresInSeconds: num(json, 'expires_in'),
        metadata: { team: team?.['id'] ?? null, scope: str(json, 'scope') },
      };
    },
  },
  {
    source: 'gmail',
    kind: 'oauth',
    credentialKey: 'GOOGLE',
    authorizeUrl: 'https://accounts.google.com/o/oauth2/v2/auth',
    tokenUrl: 'https://oauth2.googleapis.com/token',
    scope: 'https://www.googleapis.com/auth/gmail.readonly',
    usePkce: true,
    tokenAuth: 'body',
    authParams: { access_type: 'offline', prompt: 'consent' },
    extract: standardExtract,
  },
  {
    source: 'gdrive',
    kind: 'oauth',
    credentialKey: 'GOOGLE',
    authorizeUrl: 'https://accounts.google.com/o/oauth2/v2/auth',
    tokenUrl: 'https://oauth2.googleapis.com/token',
    scope: 'https://www.googleapis.com/auth/drive.readonly',
    usePkce: true,
    tokenAuth: 'body',
    authParams: { access_type: 'offline', prompt: 'consent' },
    extract: standardExtract,
  },
  {
    source: 'outlook',
    kind: 'oauth',
    credentialKey: 'MICROSOFT',
    authorizeUrl:
      'https://login.microsoftonline.com/common/oauth2/v2.0/authorize',
    tokenUrl: 'https://login.microsoftonline.com/common/oauth2/v2.0/token',
    scope: 'offline_access Mail.Read',
    usePkce: true,
    tokenAuth: 'body',
    authParams: {},
    extract: standardExtract,
  },
  {
    source: 'teams',
    kind: 'oauth',
    credentialKey: 'MICROSOFT',
    authorizeUrl:
      'https://login.microsoftonline.com/common/oauth2/v2.0/authorize',
    tokenUrl: 'https://login.microsoftonline.com/common/oauth2/v2.0/token',
    scope: 'offline_access ChannelMessage.Read.All',
    usePkce: true,
    tokenAuth: 'body',
    authParams: {},
    extract: standardExtract,
  },
  {
    source: 'notion',
    kind: 'oauth',
    credentialKey: 'NOTION',
    authorizeUrl: 'https://api.notion.com/v1/oauth/authorize',
    tokenUrl: 'https://api.notion.com/v1/oauth/token',
    scope: '',
    usePkce: false,
    tokenAuth: 'basic',
    authParams: { owner: 'user' },
    extract: (json) => ({
      accessToken: str(json, 'access_token') ?? '',
      refreshToken: null,
      expiresInSeconds: null,
      metadata: {
        workspace_id: str(json, 'workspace_id'),
        bot_id: str(json, 'bot_id'),
      },
    }),
  },
  {
    source: 'hubspot',
    kind: 'oauth',
    credentialKey: 'HUBSPOT',
    authorizeUrl: 'https://app.hubspot.com/oauth/authorize',
    tokenUrl: 'https://api.hubapi.com/oauth/v1/token',
    scope: 'crm.objects.contacts.read crm.objects.deals.read',
    usePkce: true,
    tokenAuth: 'body',
    authParams: {},
    extract: standardExtract,
  },
  {
    source: 'linear',
    kind: 'oauth',
    credentialKey: 'LINEAR',
    authorizeUrl: 'https://linear.app/oauth/authorize',
    tokenUrl: 'https://api.linear.app/oauth/token',
    scope: 'read',
    usePkce: true,
    tokenAuth: 'body',
    authParams: {},
    extract: standardExtract,
  },
  {
    source: 'salesforce',
    kind: 'oauth',
    credentialKey: 'SALESFORCE',
    authorizeUrl: 'https://login.salesforce.com/services/oauth2/authorize',
    tokenUrl: 'https://login.salesforce.com/services/oauth2/token',
    scope: 'api refresh_token',
    usePkce: true,
    tokenAuth: 'body',
    authParams: {},
    extract: standardExtract,
  },
];

export const APIKEY_PROVIDERS: ApiKeyProvider[] = [
  { source: 'fathom', kind: 'apikey', label: 'Fathom', needsBaseUrl: false },
  { source: 'odoo', kind: 'apikey', label: 'Odoo', needsBaseUrl: true },
];

export function providerFor(source: string): Provider | undefined {
  return (
    OAUTH_PROVIDERS.find((p) => p.source === source) ??
    APIKEY_PROVIDERS.find((p) => p.source === source)
  );
}
