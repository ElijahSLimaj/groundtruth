import { NextResponse } from 'next/server';
import type { NextRequest } from 'next/server';

import { personForEmail } from '../../../../lib/session';
import { createSupabaseServerClient } from '../../../../lib/supabase/server';

export async function GET(request: NextRequest): Promise<NextResponse> {
  const code = request.nextUrl.searchParams.get('code');
  if (!code) {
    return NextResponse.redirect(
      new URL('/login?error=missing_code', request.url),
    );
  }

  const supabase = await createSupabaseServerClient();
  const { data, error } = await supabase.auth.exchangeCodeForSession(code);
  if (error || !data.user?.email) {
    return NextResponse.redirect(
      new URL('/login?error=exchange_failed', request.url),
    );
  }

  const person = await personForEmail(data.user.email);
  if (!person) {
    await supabase.auth.signOut();
    return NextResponse.redirect(
      new URL('/login?error=no_account', request.url),
    );
  }

  return NextResponse.redirect(new URL('/drift', request.url));
}
