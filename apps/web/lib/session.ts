export interface Viewer {
  tenantId: string;
  personId: string;
  role: string;
  displayName: string;
}

const SEED_TENANT = '11111111-1111-1111-1111-111111111111';
const SEED_FOUNDER = '22222222-0000-0000-0000-000000000001';

export function getViewer(): Viewer {
  return {
    tenantId: process.env.DEV_TENANT_ID ?? SEED_TENANT,
    personId: process.env.DEV_PERSON_ID ?? SEED_FOUNDER,
    role: process.env.DEV_PERSON_ROLE ?? 'admin',
    displayName: process.env.DEV_PERSON_NAME ?? 'Ada Founder',
  };
}
