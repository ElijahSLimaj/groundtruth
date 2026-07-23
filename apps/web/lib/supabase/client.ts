import { createBrowserClient } from '@supabase/ssr';

export function createSupabaseBrowserClient(
  url: string,
  publishableKey: string,
) {
  return createBrowserClient(url, publishableKey);
}
