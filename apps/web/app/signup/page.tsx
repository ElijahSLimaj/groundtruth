import Link from 'next/link';

import { BrandMark } from '../../components/brand-mark';
import { startCheckout } from './actions';

export const dynamic = 'force-dynamic';

const PLANS = [
  { plan: 'core', label: 'Core', blurb: 'Up to 50 people, 4 connectors' },
  { plan: 'growth', label: 'Growth', blurb: 'Up to 300 people, all connectors, SSO' },
  { plan: 'scale', label: 'Scale', blurb: 'Custom retention, audit exports, dedicated' },
];

const ERROR_MESSAGES: Record<string, string> = {
  missing: 'Fill in your name, work email, and company.',
  billing: 'Could not start checkout. Billing may not be configured yet.',
  canceled: 'Checkout was canceled. Pick a plan to try again.',
};

export default async function SignupPage({
  searchParams,
}: {
  searchParams: Promise<{ error?: string; canceled?: string }>;
}) {
  const { error, canceled } = await searchParams;
  const message = canceled
    ? ERROR_MESSAGES.canceled
    : error
      ? (ERROR_MESSAGES[error] ?? null)
      : null;

  return (
    <div className="mx-auto flex min-h-screen max-w-lg flex-col justify-center px-6">
      <div className="rounded-card border border-line bg-surface px-8 py-10">
        <span className="flex items-center gap-2.5">
          <BrandMark size={30} />
          <span className="eyebrow text-ink-muted">Groundtruth</span>
        </span>
        <h1 className="mt-4 font-display font-extrabold text-2xl text-ink">
          Start your pilot
        </h1>
        <p className="mt-2 text-sm text-ink-muted">
          30 days, full product, cold start included. Your card is on file; you
          are not charged until the pilot converts.
        </p>

        {message ? (
          <p
            role="alert"
            className="mt-4 rounded-card border border-conflict/40 bg-conflict/10 px-4 py-3 text-sm text-conflict"
          >
            {message}
          </p>
        ) : null}

        <form action={startCheckout} className="mt-6 flex flex-col gap-3">
          <input
            name="name"
            required
            placeholder="Your name"
            className="rounded-control border border-line bg-void px-4 py-3 text-sm text-ink placeholder:text-ink-muted"
          />
          <input
            name="email"
            type="email"
            required
            placeholder="you@company.com"
            className="rounded-control border border-line bg-void px-4 py-3 text-sm text-ink placeholder:text-ink-muted"
          />
          <input
            name="company"
            required
            placeholder="Company name"
            className="rounded-control border border-line bg-void px-4 py-3 text-sm text-ink placeholder:text-ink-muted"
          />

          <fieldset className="mt-2 flex flex-col gap-2">
            <legend className="eyebrow text-ink-muted mb-1">Plan</legend>
            {PLANS.map((p, i) => (
              <label
                key={p.plan}
                className="flex items-start gap-3 rounded-control border border-line px-3 py-2 has-[:checked]:border-action"
              >
                <input
                  type="radio"
                  name="plan"
                  value={p.plan}
                  defaultChecked={i === 1}
                  className="mt-1"
                />
                <span>
                  <span className="text-sm text-ink">{p.label}</span>
                  <span className="block text-xs text-ink-muted">{p.blurb}</span>
                </span>
              </label>
            ))}
          </fieldset>

          <div className="mt-1 flex gap-4 text-sm text-ink-secondary">
            <label className="flex items-center gap-2">
              <input type="radio" name="interval" value="month" defaultChecked />
              Monthly
            </label>
            <label className="flex items-center gap-2">
              <input type="radio" name="interval" value="year" />
              Yearly
            </label>
          </div>

          <button
            type="submit"
            className="mt-3 rounded-control bg-action px-4 py-3 text-center text-sm font-medium text-void hover:opacity-90"
          >
            Continue to checkout
          </button>
        </form>

        <p className="mt-6 text-xs text-ink-muted">
          Already have an account?{' '}
          <Link href="/login" className="text-action underline">
            Sign in
          </Link>
        </p>
      </div>
    </div>
  );
}
