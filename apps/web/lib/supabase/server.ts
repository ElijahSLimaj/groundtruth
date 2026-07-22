import { createServerClient } from '@supabase/ssr';
import { cookies } from 'next/headers';

import { supabasePublishableKey, supabaseUrl } from './config';

export async function createSupabaseServerClient() {
  const cookieStore = await cookies();
  return createServerClient(supabaseUrl(), supabasePublishableKey(), {
    cookies: {
      getAll() {
        return cookieStore.getAll();
      },
      setAll(cookiesToSet) {
        try {
          cookiesToSet.forEach(({ name, value, options }) =>
            cookieStore.set(name, value, options),
          );
        } catch {
          return;
        }
      },
    },
  });
}
