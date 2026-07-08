import { cookies } from 'next/headers';
import { NextResponse } from 'next/server';
import type { NextRequest } from 'next/server';

import { destroySession, SESSION_COOKIE } from '../../../../lib/session';

export async function POST(request: NextRequest): Promise<NextResponse> {
  const store = await cookies();
  const token = store.get(SESSION_COOKIE)?.value;
  if (token) {
    await destroySession(token);
  }
  const response = NextResponse.redirect(new URL('/login', request.url), 303);
  response.cookies.delete(SESSION_COOKIE);
  return response;
}
