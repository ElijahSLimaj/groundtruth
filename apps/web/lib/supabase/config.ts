export function supabaseUrl(): string {
  const value = process.env.NEXT_PUBLIC_SUPABASE_URL;
  if (!value) {
    throw new Error('NEXT_PUBLIC_SUPABASE_URL is required');
  }
  return value;
}

export function supabasePublishableKey(): string {
  const value = process.env.NEXT_PUBLIC_SUPABASE_PUBLISHABLE_KEY;
  if (!value) {
    throw new Error('NEXT_PUBLIC_SUPABASE_PUBLISHABLE_KEY is required');
  }
  return value;
}
