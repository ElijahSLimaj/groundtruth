import { createCipheriv, createDecipheriv, randomBytes } from 'node:crypto';
import type { PoolClient } from 'pg';

const NONCE_BYTES = 12;
const TAG_BYTES = 16;
const DATA_KEY_BYTES = 32;

export function masterKey(hex: string): Buffer {
  const key = Buffer.from(hex, 'hex');
  if (key.length !== DATA_KEY_BYTES) {
    throw new Error(`MASTER_KEY must be ${DATA_KEY_BYTES} bytes of hex`);
  }
  return key;
}

export function gcmSeal(key: Buffer, plaintext: Buffer, aad?: Buffer): Buffer {
  const nonce = randomBytes(NONCE_BYTES);
  const cipher = createCipheriv('aes-256-gcm', key, nonce);
  if (aad) {
    cipher.setAAD(aad);
  }
  const ciphertext = Buffer.concat([cipher.update(plaintext), cipher.final()]);
  return Buffer.concat([nonce, ciphertext, cipher.getAuthTag()]);
}

export function gcmOpen(key: Buffer, sealed: Buffer, aad?: Buffer): Buffer {
  if (sealed.length < NONCE_BYTES + TAG_BYTES) {
    throw new Error('sealed value too short to be an envelope');
  }
  const nonce = sealed.subarray(0, NONCE_BYTES);
  const tag = sealed.subarray(sealed.length - TAG_BYTES);
  const ciphertext = sealed.subarray(NONCE_BYTES, sealed.length - TAG_BYTES);
  const decipher = createDecipheriv('aes-256-gcm', key, nonce);
  if (aad) {
    decipher.setAAD(aad);
  }
  decipher.setAuthTag(tag);
  return Buffer.concat([decipher.update(ciphertext), decipher.final()]);
}

function secretAad(connectorId: string): Buffer {
  return Buffer.from(`connector:${connectorId}`);
}

export function sealConnectorSecret(
  dataKey: Buffer,
  connectorId: string,
  secret: unknown,
): string {
  const plaintext = Buffer.from(JSON.stringify(secret));
  return gcmSeal(dataKey, plaintext, secretAad(connectorId)).toString('base64');
}

export function openConnectorSecret<T>(
  dataKey: Buffer,
  connectorId: string,
  sealedB64: string,
): T {
  const opened = gcmOpen(
    dataKey,
    Buffer.from(sealedB64, 'base64'),
    secretAad(connectorId),
  );
  return JSON.parse(opened.toString('utf8')) as T;
}

export async function tenantDataKey(
  client: PoolClient,
  master: Buffer,
): Promise<Buffer> {
  const existing = await client.query<{ wrapped_key: Buffer }>(
    `select wrapped_key from tenant_keys where tenant_id = public.app_tenant_id()`,
  );
  if (existing.rows[0]) {
    return gcmOpen(master, existing.rows[0].wrapped_key);
  }
  const fresh = randomBytes(DATA_KEY_BYTES);
  const wrapped = gcmSeal(master, fresh);
  await client.query(
    `insert into tenant_keys (tenant_id, wrapped_key)
     values (public.app_tenant_id(), $1)
     on conflict (tenant_id) do nothing`,
    [wrapped],
  );
  const reread = await client.query<{ wrapped_key: Buffer }>(
    `select wrapped_key from tenant_keys where tenant_id = public.app_tenant_id()`,
  );
  return gcmOpen(master, reread.rows[0].wrapped_key);
}
