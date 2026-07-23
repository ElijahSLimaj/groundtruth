import { NextResponse } from 'next/server';
import type { NextRequest } from 'next/server';

import { publicOrigin } from '../../../../lib/request-origin';
import { personForEmail } from '../../../../lib/session';
import { createSupabaseServerClient } from '../../../../lib/supabase/server';

export async function GET(request: NextRequest): Promise<NextResponse> {
  const origin = publicOrigin(request);
  const code = request.nextUrl.searchParams.get('code');
  if (!code) {
    return NextResponse.redirect(new URL('/login?error=missing_code', origin));
  }

  const supabase = await createSupabaseServerClient();
  const { data, error } = await supabase.auth.exchangeCodeForSession(code);
  if (error || !data.user?.email) {
    return NextResponse.redirect(
      new URL('/login?error=exchange_failed', origin),
    );
  }

  const person = await personForEmail(data.user.email);
  if (!person) {
    await supabase.auth.signOut();
    return NextResponse.redirect(new URL('/login?error=no_account', origin));
  }

  return NextResponse.redirect(new URL('/drift', origin));
}
