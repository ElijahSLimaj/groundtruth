'use server';

import { revalidatePath } from 'next/cache';

import { brainFetch } from '../../../lib/brain-api';
import { requireViewer } from '../../../lib/session';

export async function connectApiKey(formData: FormData): Promise<void> {
  const viewer = await requireViewer();
  const source = String(formData.get('source') ?? '');
  const apiKey = String(formData.get('api_key') ?? '');
  const baseUrl = String(formData.get('base_url') ?? '');
  if (!source || !apiKey) {
    return;
  }
  await brainFetch(viewer, '/connectors/apikey', {
    method: 'POST',
    body: {
      source,
      api_key: apiKey,
      base_url: baseUrl.length > 0 ? baseUrl : undefined,
    },
  });
  revalidatePath('/connectors');
}

export async function disconnectConnector(formData: FormData): Promise<void> {
  const viewer = await requireViewer();
  const id = String(formData.get('id') ?? '');
  if (!id) {
    return;
  }
  await brainFetch(viewer, `/connectors/${id}`, { method: 'DELETE' });
  revalidatePath('/connectors');
}
