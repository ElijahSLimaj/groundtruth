import { createBrowserClient } from '@supabase/ssr';

import { supabasePublishableKey, supabaseUrl } from './config';

export function createSupabaseBrowserClient() {
  return createBrowserClient(supabaseUrl(), supabasePublishableKey());
}
