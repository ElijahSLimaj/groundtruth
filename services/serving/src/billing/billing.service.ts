import {
  BadRequestException,
  Inject,
  Injectable,
  Logger,
  ServiceUnavailableException,
} from '@nestjs/common';
import Stripe from 'stripe';

import { SERVING_CONFIG } from '../config';
import type { ServingConfig } from '../config';
import { DatabaseService } from '../database/database.service';

export interface CheckoutInput {
  email: string;
  name: string;
  company: string;
  plan: string;
  interval: 'month' | 'year';
}

@Injectable()
export class BillingService {
  private readonly logger = new Logger(BillingService.name);
  private readonly stripe: Stripe | null;
  readonly enabled: boolean;

  constructor(
    private readonly db: DatabaseService,
    @Inject(SERVING_CONFIG) private readonly config: ServingConfig,
  ) {
    this.enabled = Boolean(config.stripeSecretKey);
    this.stripe = config.stripeSecretKey
      ? new Stripe(config.stripeSecretKey)
      : null;
  }

  private client(): Stripe {
    if (!this.stripe) {
      throw new ServiceUnavailableException(
        'billing is disabled until STRIPE_SECRET_KEY is configured',
      );
    }
    return this.stripe;
  }

  private appUrl(): string {
    if (!this.config.appUrl) {
      throw new BadRequestException('APP_URL is not configured');
    }
    return this.config.appUrl.replace(/\/$/, '');
  }

  private async priceId(
    plan: string,
    interval: 'month' | 'year',
  ): Promise<string> {
    const column =
      interval === 'year' ? 'stripe_price_yearly' : 'stripe_price_monthly';
    const price = await this.db.asApp(async (client) => {
      const rows = await client.query<{ price: string | null }>(
        `select ${column} as price from plan_limits where plan = $1`,
        [plan],
      );
      return rows.rows[0]?.price ?? null;
    });
    if (!price) {
      throw new BadRequestException(
        `no Stripe price configured for ${plan}/${interval}; set plan_limits.${column}`,
      );
    }
    return price;
  }

  async createCheckout(input: CheckoutInput): Promise<{ url: string }> {
    const stripe = this.client();
    const price = await this.priceId(input.plan, input.interval);
    const session = await stripe.checkout.sessions.create({
      mode: 'subscription',
      customer_email: input.email,
      line_items: [{ price, quantity: 1 }],
      subscription_data: { trial_period_days: 30 },
      metadata: {
        email: input.email,
        name: input.name,
        company: input.company,
        plan: input.plan,
        interval: input.interval,
      },
      success_url: `${this.appUrl()}/login?provisioned=1`,
      cancel_url: `${this.appUrl()}/signup?canceled=1`,
    });
    if (!session.url) {
      throw new BadRequestException('Stripe did not return a checkout url');
    }
    return { url: session.url };
  }

  async createPortal(tenantId: string): Promise<{ url: string }> {
    const stripe = this.client();
    const customerId = await this.db.withTenant(tenantId, async (client) => {
      const rows = await client.query<{ stripe_customer_id: string | null }>(
        `select stripe_customer_id from tenants where id = $1`,
        [tenantId],
      );
      return rows.rows[0]?.stripe_customer_id ?? null;
    });
    if (!customerId) {
      throw new BadRequestException('tenant has no Stripe customer');
    }
    const session = await stripe.billingPortal.sessions.create({
      customer: customerId,
      return_url: `${this.appUrl()}/settings`,
    });
    return { url: session.url };
  }

  async handleWebhook(rawBody: Buffer, signature: string): Promise<void> {
    const stripe = this.client();
    if (!this.config.stripeWebhookSecret) {
      throw new ServiceUnavailableException('STRIPE_WEBHOOK_SECRET is not set');
    }
    let event: Stripe.Event;
    try {
      event = stripe.webhooks.constructEvent(
        rawBody,
        signature,
        this.config.stripeWebhookSecret,
      );
    } catch (error) {
      throw new BadRequestException(
        `invalid stripe signature: ${String(error)}`,
      );
    }

    if (event.type === 'checkout.session.completed') {
      await this.onCheckoutCompleted(event.data.object);
    } else if (
      event.type === 'customer.subscription.updated' ||
      event.type === 'customer.subscription.deleted'
    ) {
      await this.onSubscriptionChanged(event.data.object);
    }
  }

  private async onCheckoutCompleted(
    session: Stripe.Checkout.Session,
  ): Promise<void> {
    const meta = session.metadata ?? {};
    const email = meta.email;
    if (!email) {
      this.logger.warn('checkout.session.completed without email metadata');
      return;
    }
    const plan = meta.plan ?? 'core';
    const interval = meta.interval === 'year' ? 'year' : 'month';
    await this.db.asApp(async (client) => {
      const provisioned = await client.query<{
        tenant_id: string;
      }>(`select tenant_id from public.provision_tenant($1, $2, $3, $4, $5)`, [
        email,
        meta.name ?? email,
        meta.company ?? email,
        plan,
        interval,
      ]);
      const tenantId = provisioned.rows[0].tenant_id;
      await client.query(
        `select public.tenant_set_subscription($1, $2, $3, 'active', $4, $5)`,
        [
          tenantId,
          typeof session.customer === 'string' ? session.customer : null,
          typeof session.subscription === 'string'
            ? session.subscription
            : null,
          plan,
          interval,
        ],
      );
    });
    this.logger.log(
      JSON.stringify({ event: 'tenant_provisioned_via_checkout', email }),
    );
  }

  private async onSubscriptionChanged(
    subscription: Stripe.Subscription,
  ): Promise<void> {
    await this.db.asApp(async (client) => {
      await client.query(
        `update tenants set subscription_status = $2
         where stripe_subscription_id = $1`,
        [subscription.id, subscription.status],
      );
    });
  }
}
