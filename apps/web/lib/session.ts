import { createHash, randomBytes } from 'node:crypto';

import { cookies } from 'next/headers';
import { redirect } from 'next/navigation';

import { withApp, withTenant } from './db';
import { oidcConfig } from './oidc';

export interface Viewer {
  tenantId: string;
  personId: string;
  role: string;
  displayName: string;
}

export const SESSION_COOKIE = 'brain_session';
const SESSION_TTL_HOURS = 12;

const SEED_TENANT = '11111111-1111-1111-1111-111111111111';
const SEED_FOUNDER = '22222222-0000-0000-0000-000000000001';

export function devModeEnabled(): boolean {
  return oidcConfig() === null && process.env.NODE_ENV !== 'production';
}

function devViewer(): Viewer {
  return {
    tenantId: process.env.DEV_TENANT_ID ?? SEED_TENANT,
    personId: process.env.DEV_PERSON_ID ?? SEED_FOUNDER,
    role: process.env.DEV_PERSON_ROLE ?? 'admin',
    displayName: process.env.DEV_PERSON_NAME ?? 'Ada Founder',
  };
}

const hashToken = (token: string) =>
  createHash('sha256').update(token).digest('hex');

export async function getViewer(): Promise<Viewer | null> {
  const store = await cookies();
  const token = store.get(SESSION_COOKIE)?.value;
  if (!token) {
    return devModeEnabled() ? devViewer() : null;
  }
  const rows = await withApp((client) =>
    client.query<{
      tenant_id: string;
      person_id: string;
      role: string;
      display_name: string;
    }>(`select * from public.web_session_lookup($1)`, [hashToken(token)]),
  );
  const row = rows.rows[0];
  if (!row) {
    return devModeEnabled() ? devViewer() : null;
  }
  return {
    tenantId: row.tenant_id,
    personId: row.person_id,
    role: row.role,
    displayName: row.display_name,
  };
}

export async function requireViewer(): Promise<Viewer> {
  const viewer = await getViewer();
  if (!viewer) {
    redirect('/login');
  }
  return viewer;
}

export async function createSession(
  tenantId: string,
  personId: string,
): Promise<{ token: string; maxAgeSeconds: number }> {
  const token = randomBytes(32).toString('hex');
  await withTenant(tenantId, (client) =>
    client.query(
      `insert into web_sessions (tenant_id, person_id, token_hash, expires_at)
       values ($1, $2, $3, now() + make_interval(hours => $4))`,
      [tenantId, personId, hashToken(token), SESSION_TTL_HOURS],
    ),
  );
  return { token, maxAgeSeconds: SESSION_TTL_HOURS * 3600 };
}

export async function destroySession(token: string): Promise<void> {
  await withApp((client) =>
    client.query(`select public.web_session_destroy($1)`, [hashToken(token)]),
  );
}

export async function findPersonByEmail(
  email: string,
): Promise<{ tenantId: string; personId: string } | null> {
  const rows = await withApp((client) =>
    client.query<{ tenant_id: string; person_id: string }>(
      `select * from public.web_person_by_email($1)`,
      [email],
    ),
  );
  const row = rows.rows[0];
  return row ? { tenantId: row.tenant_id, personId: row.person_id } : null;
}
