'use server';

import { redirect } from 'next/navigation';

import { brainFetchPublic } from '../../lib/brain-api';

export async function startCheckout(formData: FormData): Promise<void> {
  const payload = {
    email: String(formData.get('email') ?? ''),
    name: String(formData.get('name') ?? ''),
    company: String(formData.get('company') ?? ''),
    plan: String(formData.get('plan') ?? 'core'),
    interval: String(formData.get('interval') ?? 'month'),
  };
  if (!payload.email || !payload.name || !payload.company) {
    redirect('/signup?error=missing');
  }

  let url: string;
  try {
    const result = await brainFetchPublic<{ url: string }>(
      '/billing/checkout',
      payload,
    );
    url = result.url;
  } catch {
    redirect('/signup?error=billing');
  }
  redirect(url);
}
