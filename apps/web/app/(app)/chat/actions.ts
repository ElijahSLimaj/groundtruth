'use server';

import { redirect } from 'next/navigation';

import { brainFetch } from '../../../lib/brain-api';
import { requireViewer } from '../../../lib/session';

export async function sendMessage(formData: FormData): Promise<void> {
  const viewer = await requireViewer();
  const content = String(formData.get('content') ?? '').trim();
  const conversationId = String(formData.get('conversation_id') ?? '');
  if (!content) {
    return;
  }
  const result = await brainFetch<{ conversation_id: string }>(
    viewer,
    '/chat/messages',
    {
      method: 'POST',
      body: {
        content,
        conversation_id: conversationId || undefined,
      },
    },
  );
  redirect(`/chat?c=${result.conversation_id}`);
}
