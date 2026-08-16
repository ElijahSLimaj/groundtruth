import { ServiceUnavailableException } from '@nestjs/common';

import type { ServingConfig } from '../config';
import type { DatabaseService } from '../database/database.service';
import { BillingService } from './billing.service';

const db = {} as DatabaseService;

describe('BillingService (disabled without Stripe)', () => {
  const svc = new BillingService(db, {
    stripeSecretKey: null,
    appUrl: 'https://app.test',
  } as unknown as ServingConfig);

  it('reports itself disabled', () => {
    expect(svc.enabled).toBe(false);
  });

  it('refuses checkout until Stripe is configured', async () => {
    await expect(
      svc.createCheckout({
        email: 'a@b.test',
        name: 'A',
        company: 'B',
        plan: 'core',
        interval: 'month',
      }),
    ).rejects.toBeInstanceOf(ServiceUnavailableException);
  });

  it('reports enabled when a key is present', () => {
    const enabled = new BillingService(db, {
      stripeSecretKey: 'sk_test_x',
      appUrl: 'https://app.test',
    } as unknown as ServingConfig);
    expect(enabled.enabled).toBe(true);
  });
});
