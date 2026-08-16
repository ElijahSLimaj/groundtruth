import type { Viewer } from './session';

export function chatConfigured(): boolean {
  return Boolean(process.env.SERVING_URL && process.env.INTERNAL_API_SECRET);
}

export async function brainFetch<T>(
  viewer: Viewer,
  path: string,
  init?: { method?: string; body?: unknown },
): Promise<T> {
  const base = process.env.SERVING_URL;
  const secret = process.env.INTERNAL_API_SECRET;
  if (!base || !secret) {
    throw new Error('SERVING_URL and INTERNAL_API_SECRET are required');
  }
  const response = await fetch(base.replace(/\/$/, '') + path, {
    method: init?.method ?? 'GET',
    headers: {
      'content-type': 'application/json',
      'x-internal-secret': secret,
      'x-tenant-id': viewer.tenantId,
      'x-person-id': viewer.personId,
    },
    body: init?.body === undefined ? undefined : JSON.stringify(init.body),
    cache: 'no-store',
  });
  if (!response.ok) {
    const body = await response.text();
    throw new Error(`brain api ${path}: ${response.status} ${body.slice(0, 200)}`);
  }
  return (await response.json()) as T;
}

export async function brainFetchPublic<T>(
  path: string,
  body: unknown,
): Promise<T> {
  const base = process.env.SERVING_URL;
  const secret = process.env.INTERNAL_API_SECRET;
  if (!base || !secret) {
    throw new Error('SERVING_URL and INTERNAL_API_SECRET are required');
  }
  const response = await fetch(base.replace(/\/$/, '') + path, {
    method: 'POST',
    headers: {
      'content-type': 'application/json',
      'x-internal-secret': secret,
    },
    body: JSON.stringify(body),
    cache: 'no-store',
  });
  if (!response.ok) {
    const text = await response.text();
    throw new Error(`brain api ${path}: ${response.status} ${text.slice(0, 200)}`);
  }
  return (await response.json()) as T;
}
