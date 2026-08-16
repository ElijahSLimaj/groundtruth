'use server';

import { redirect } from 'next/navigation';
import { revalidatePath } from 'next/cache';

import { brainFetch } from '../../../lib/brain-api';
import { requireViewer } from '../../../lib/session';

export async function issueKey(formData: FormData): Promise<void> {
  const viewer = await requireViewer();
  const name = String(formData.get('name') ?? '').trim();
  const rateTier = String(formData.get('rate_tier') ?? 'standard');
  if (!name) {
    return;
  }
  const result = await brainFetch<{ id: string; key: string }>(
    viewer,
    '/account/keys',
    { method: 'POST', body: { name, rate_tier: rateTier } },
  );
  revalidatePath('/settings');
  redirect(`/settings?new_key=${encodeURIComponent(result.key)}`);
}

export async function revokeKey(formData: FormData): Promise<void> {
  const viewer = await requireViewer();
  const id = String(formData.get('id') ?? '');
  if (!id) {
    return;
  }
  await brainFetch(viewer, `/account/keys/${id}`, { method: 'DELETE' });
  revalidatePath('/settings');
}

export async function openBillingPortal(): Promise<void> {
  const viewer = await requireViewer();
  let url: string;
  try {
    const result = await brainFetch<{ url: string }>(viewer, '/billing/portal', {
      method: 'POST',
    });
    url = result.url;
  } catch {
    redirect('/settings?error=billing');
  }
  redirect(url);
}
