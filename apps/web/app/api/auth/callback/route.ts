import { cookies } from 'next/headers';
import { NextResponse } from 'next/server';
import type { NextRequest } from 'next/server';

import {
  exchangeCodeForEmail,
  FLOW_COOKIE,
  oidcConfig,
} from '../../../../lib/oidc';
import {
  createSession,
  findPersonByEmail,
  SESSION_COOKIE,
} from '../../../../lib/session';

export async function GET(request: NextRequest): Promise<NextResponse> {
  const config = oidcConfig();
  if (!config) {
    return NextResponse.json(
      { error: 'sso is not configured' },
      { status: 503 },
    );
  }
  const store = await cookies();
  const flowRaw = store.get(FLOW_COOKIE)?.value;
  store.delete(FLOW_COOKIE);

  const code = request.nextUrl.searchParams.get('code');
  const state = request.nextUrl.searchParams.get('state');
  if (!flowRaw || !code || !state) {
    return NextResponse.redirect(
      new URL('/login?error=missing_flow', request.url),
    );
  }
  let flow: { state: string; nonce: string; verifier: string };
  try {
    flow = JSON.parse(flowRaw) as typeof flow;
  } catch {
    return NextResponse.redirect(
      new URL('/login?error=missing_flow', request.url),
    );
  }
  if (flow.state !== state) {
    return NextResponse.redirect(
      new URL('/login?error=state_mismatch', request.url),
    );
  }

  let email: string;
  try {
    email = await exchangeCodeForEmail(config, code, flow.verifier, flow.nonce);
  } catch {
    return NextResponse.redirect(
      new URL('/login?error=exchange_failed', request.url),
    );
  }

  const person = await findPersonByEmail(email);
  if (!person) {
    return NextResponse.redirect(
      new URL('/login?error=no_account', request.url),
    );
  }

  const session = await createSession(person.tenantId, person.personId);
  const response = NextResponse.redirect(new URL('/', request.url));
  response.cookies.set(SESSION_COOKIE, session.token, {
    httpOnly: true,
    sameSite: 'lax',
    secure: process.env.NODE_ENV === 'production',
    path: '/',
    maxAge: session.maxAgeSeconds,
  });
  return response;
}
