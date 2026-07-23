import type { NextRequest } from 'next/server';

export function publicOrigin(request: NextRequest): string {
  const forwardedProto = request.headers.get('x-forwarded-proto');
  const forwardedHost = request.headers.get('x-forwarded-host');
  const host = forwardedHost ?? request.headers.get('host');
  if (!host) {
    return request.nextUrl.origin;
  }
  const proto =
    forwardedProto ?? (host.startsWith('localhost') ? 'http' : 'https');
  return `${proto}://${host}`;
}
