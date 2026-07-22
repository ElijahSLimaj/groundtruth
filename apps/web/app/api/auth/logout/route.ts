import { NextResponse } from 'next/server';
import type { NextRequest } from 'next/server';

import { createSupabaseServerClient } from '../../../../lib/supabase/server';

export async function POST(request: NextRequest): Promise<NextResponse> {
  const supabase = await createSupabaseServerClient();
  await supabase.auth.signOut();
  return NextResponse.redirect(new URL('/login', request.url), 303);
}
