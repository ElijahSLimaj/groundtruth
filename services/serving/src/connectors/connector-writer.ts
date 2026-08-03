import { PoolClient } from 'pg';

import { DatabaseService } from '../database/database.service';
import { masterKey, sealConnectorSecret, tenantDataKey } from './secret-crypto';

export interface ConnectResult {
  connector_id: string;
  source_type: string;
  status: string;
}

async function upsertConnector(
  client: PoolClient,
  tenantId: string,
  source: string,
): Promise<string> {
  const inserted = await client.query<{ id: string }>(
    `insert into connectors (tenant_id, source_type, status, config)
     values ($1, $2, 'connecting', '{}'::jsonb)
     on conflict (tenant_id, source_type)
     do update set status = 'connecting'
     returning id`,
    [tenantId, source],
  );
  return inserted.rows[0].id;
}

export async function writeConnectorSecret(params: {
  db: DatabaseService;
  masterKeyHex: string;
  tenantId: string;
  source: string;
  secret: Record<string, unknown>;
  clearConfig: Record<string, unknown>;
}): Promise<ConnectResult> {
  const master = masterKey(params.masterKeyHex);
  return params.db.withTenant(params.tenantId, async (client) => {
    const connectorId = await upsertConnector(
      client,
      params.tenantId,
      params.source,
    );
    const dataKey = await tenantDataKey(client, master);
    const sealed = sealConnectorSecret(dataKey, connectorId, params.secret);
    await client.query(
      `update connectors set config = $2, status = 'live' where id = $1`,
      [connectorId, JSON.stringify({ ...params.clearConfig, secret: sealed })],
    );
    return {
      connector_id: connectorId,
      source_type: params.source,
      status: 'live',
    };
  });
}
