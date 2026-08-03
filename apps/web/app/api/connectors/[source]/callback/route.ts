import { NextResponse } from 'next/server';
import type { NextRequest } from 'next/server';

import { brainFetch } from '../../../../../lib/brain-api';
import { publicOrigin } from '../../../../../lib/request-origin';
import { getViewer } from '../../../../../lib/session';

export async function GET(
  request: NextRequest,
  context: { params: Promise<{ source: string }> },
): Promise<NextResponse> {
  const origin = publicOrigin(request);
  const { source } = await context.params;
  const code = request.nextUrl.searchParams.get('code');
  const state = request.nextUrl.searchParams.get('state');

  const viewer = await getViewer();
  if (!viewer) {
    return NextResponse.redirect(new URL('/login', origin));
  }
  if (!code || !state) {
    return NextResponse.redirect(
      new URL('/connectors?error=oauth_denied', origin),
    );
  }
  try {
    await brainFetch(viewer, '/connectors/oauth/callback', {
      method: 'POST',
      body: { state, code },
    });
    return NextResponse.redirect(
      new URL(`/connectors?connected=${source}`, origin),
    );
  } catch {
    return NextResponse.redirect(
      new URL('/connectors?error=connect_failed', origin),
    );
  }
}
