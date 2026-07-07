'use client';

import { useEffect } from 'react';

export function QueueKeys() {
  useEffect(() => {
    const handler = (event: KeyboardEvent) => {
      if (
        event.target instanceof HTMLInputElement ||
        event.target instanceof HTMLTextAreaElement ||
        event.target instanceof HTMLSelectElement ||
        event.metaKey ||
        event.ctrlKey
      ) {
        return;
      }
      if (event.key === 'a' || event.key === 'A') {
        document.getElementById('queue-approve')?.click();
      }
      if (event.key === 'x' || event.key === 'X') {
        document.getElementById('queue-reject-toggle')?.click();
      }
    };
    document.addEventListener('keydown', handler);
    return () => document.removeEventListener('keydown', handler);
  }, []);
  return null;
}
