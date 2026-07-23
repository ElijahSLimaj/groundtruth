export function supabaseUrl(): string {
  const value = process.env.SUPABASE_URL;
  if (!value) {
    throw new Error('SUPABASE_URL is required');
  }
  return value;
}

export function supabasePublishableKey(): string {
  const value = process.env.SUPABASE_PUBLISHABLE_KEY;
  if (!value) {
    throw new Error('SUPABASE_PUBLISHABLE_KEY is required');
  }
  return value;
}
