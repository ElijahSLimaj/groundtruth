import { redirect } from 'next/navigation';

import { withApp } from './db';
import { createSupabaseServerClient } from './supabase/server';

export interface Viewer {
  tenantId: string;
  personId: string;
  role: string;
  displayName: string;
  email: string;
}

interface PersonRow {
  tenant_id: string;
  person_id: string;
  role: string;
  display_name: string;
}

export async function personForEmail(email: string): Promise<Viewer | null> {
  const rows = await withApp((client) =>
    client.query<PersonRow>('select * from public.web_person_by_email($1)', [
      email,
    ]),
  );
  const row = rows.rows[0];
  if (!row) {
    return null;
  }
  return {
    tenantId: row.tenant_id,
    personId: row.person_id,
    role: row.role,
    displayName: row.display_name,
    email,
  };
}

export async function getViewer(): Promise<Viewer | null> {
  const supabase = await createSupabaseServerClient();
  const {
    data: { user },
  } = await supabase.auth.getUser();
  const email = user?.email;
  if (!email) {
    return null;
  }
  return personForEmail(email);
}

export async function requireViewer(): Promise<Viewer> {
  const viewer = await getViewer();
  if (!viewer) {
    redirect('/login');
  }
  return viewer;
}
