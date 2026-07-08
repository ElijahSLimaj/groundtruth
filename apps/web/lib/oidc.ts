import { createRemoteJWKSet, jwtVerify } from 'jose';

export const FLOW_COOKIE = 'oidc_flow';

export interface OidcConfig {
  issuer: string;
  clientId: string;
  clientSecret: string;
  redirectUri: string;
}

interface DiscoveryDocument {
  issuer: string;
  authorization_endpoint: string;
  token_endpoint: string;
  jwks_uri: string;
}

let cachedDiscovery: { issuer: string; doc: DiscoveryDocument } | null = null;
let cachedJwks: ReturnType<typeof createRemoteJWKSet> | null = null;

export function oidcConfig(): OidcConfig | null {
  const issuer = process.env.OIDC_ISSUER;
  const clientId = process.env.OIDC_CLIENT_ID;
  const clientSecret = process.env.OIDC_CLIENT_SECRET;
  const redirectUri = process.env.OIDC_REDIRECT_URI;
  if (!issuer || !clientId || !clientSecret || !redirectUri) {
    return null;
  }
  return { issuer, clientId, clientSecret, redirectUri };
}

async function discover(issuer: string): Promise<DiscoveryDocument> {
  if (cachedDiscovery?.issuer === issuer) {
    return cachedDiscovery.doc;
  }
  const url = `${issuer.replace(/\/$/, '')}/.well-known/openid-configuration`;
  const response = await fetch(url);
  if (!response.ok) {
    throw new Error(`oidc discovery failed with status ${response.status}`);
  }
  const doc = (await response.json()) as DiscoveryDocument;
  if (!doc.authorization_endpoint || !doc.token_endpoint || !doc.jwks_uri) {
    throw new Error('oidc discovery document is missing required endpoints');
  }
  cachedDiscovery = { issuer, doc };
  cachedJwks = null;
  return doc;
}

export async function buildAuthorizationUrl(
  config: OidcConfig,
  state: string,
  nonce: string,
  codeChallenge: string,
): Promise<string> {
  const doc = await discover(config.issuer);
  const url = new URL(doc.authorization_endpoint);
  url.searchParams.set('response_type', 'code');
  url.searchParams.set('client_id', config.clientId);
  url.searchParams.set('redirect_uri', config.redirectUri);
  url.searchParams.set('scope', 'openid email profile');
  url.searchParams.set('state', state);
  url.searchParams.set('nonce', nonce);
  url.searchParams.set('code_challenge', codeChallenge);
  url.searchParams.set('code_challenge_method', 'S256');
  return url.toString();
}

export async function exchangeCodeForEmail(
  config: OidcConfig,
  code: string,
  codeVerifier: string,
  nonce: string,
): Promise<string> {
  const doc = await discover(config.issuer);
  const response = await fetch(doc.token_endpoint, {
    method: 'POST',
    headers: { 'content-type': 'application/x-www-form-urlencoded' },
    body: new URLSearchParams({
      grant_type: 'authorization_code',
      code,
      redirect_uri: config.redirectUri,
      client_id: config.clientId,
      client_secret: config.clientSecret,
      code_verifier: codeVerifier,
    }),
  });
  if (!response.ok) {
    throw new Error(`oidc token exchange failed with status ${response.status}`);
  }
  const body = (await response.json()) as { id_token?: string };
  if (!body.id_token) {
    throw new Error('oidc token response had no id_token');
  }

  if (!cachedJwks) {
    cachedJwks = createRemoteJWKSet(new URL(doc.jwks_uri));
  }
  const { payload } = await jwtVerify(body.id_token, cachedJwks, {
    issuer: doc.issuer,
    audience: config.clientId,
  });
  if (payload.nonce !== nonce) {
    throw new Error('oidc nonce mismatch');
  }
  const email = payload.email;
  if (typeof email !== 'string' || email.length === 0) {
    throw new Error('oidc id_token carried no email claim');
  }
  return email;
}
