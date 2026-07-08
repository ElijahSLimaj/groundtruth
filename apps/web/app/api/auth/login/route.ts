import { createHash, randomBytes } from 'node:crypto';

import { cookies } from 'next/headers';
import { NextResponse } from 'next/server';

import {
  buildAuthorizationUrl,
  FLOW_COOKIE,
  oidcConfig,
} from '../../../../lib/oidc';

export async function GET(): Promise<NextResponse> {
  const config = oidcConfig();
  if (!config) {
    return NextResponse.json(
      { error: 'sso is not configured' },
      { status: 503 },
    );
  }
  const state = randomBytes(16).toString('hex');
  const nonce = randomBytes(16).toString('hex');
  const verifier = randomBytes(32).toString('base64url');
  const challenge = createHash('sha256').update(verifier).digest('base64url');

  const store = await cookies();
  store.set(FLOW_COOKIE, JSON.stringify({ state, nonce, verifier }), {
    httpOnly: true,
    sameSite: 'lax',
    secure: process.env.NODE_ENV === 'production',
    path: '/',
    maxAge: 600,
  });

  const url = await buildAuthorizationUrl(config, state, nonce, challenge);
  return NextResponse.redirect(url);
}
