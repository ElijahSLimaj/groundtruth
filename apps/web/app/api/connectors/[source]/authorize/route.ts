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
  const viewer = await getViewer();
  if (!viewer) {
    return NextResponse.redirect(new URL('/login', origin));
  }
  const { source } = await context.params;
  try {
    const { authorize_url } = await brainFetch<{ authorize_url: string }>(
      viewer,
      '/connectors/oauth/start',
      { method: 'POST', body: { source } },
    );
    return NextResponse.redirect(authorize_url);
  } catch {
    return NextResponse.redirect(
      new URL(`/connectors?error=start_failed`, origin),
    );
  }
}
