import { Inject, Injectable, OnModuleDestroy } from '@nestjs/common';
import { Pool, PoolClient } from 'pg';

import { SERVING_CONFIG } from '../config';
import type { ServingConfig } from '../config';

@Injectable()
export class DatabaseService implements OnModuleDestroy {
  private readonly pool: Pool;

  constructor(@Inject(SERVING_CONFIG) config: ServingConfig) {
    this.pool = new Pool({ connectionString: config.databaseUrl, max: 10 });
  }

  async onModuleDestroy(): Promise<void> {
    await this.pool.end();
  }

  async withTenant<T>(
    tenantId: string,
    fn: (client: PoolClient) => Promise<T>,
  ): Promise<T> {
    const client = await this.pool.connect();
    try {
      await client.query('set role brain_app');
      await client.query('begin');
      await client.query("select set_config('app.tenant_id', $1, true)", [
        tenantId,
      ]);
      const result = await fn(client);
      await client.query('commit');
      return result;
    } catch (error) {
      await client.query('rollback').catch(() => undefined);
      throw error;
    } finally {
      client.release();
    }
  }

  async asApp<T>(fn: (client: PoolClient) => Promise<T>): Promise<T> {
    const client = await this.pool.connect();
    try {
      await client.query('set role brain_app');
      return await fn(client);
    } finally {
      client.release();
    }
  }

  async withAdvisoryLock(
    name: string,
    fn: () => Promise<void>,
  ): Promise<boolean> {
    const client = await this.pool.connect();
    let held = false;
    try {
      await client.query('set role brain_app');
      const acquired = await client.query<{ locked: boolean }>(
        'select pg_try_advisory_lock(hashtextextended($1, 0)) as locked',
        [name],
      );
      if (!acquired.rows[0].locked) {
        return false;
      }
      held = true;
      await fn();
      return true;
    } finally {
      if (held) {
        await client
          .query('select pg_advisory_unlock(hashtextextended($1, 0))', [name])
          .catch(() => undefined);
      }
      client.release();
    }
  }
}
