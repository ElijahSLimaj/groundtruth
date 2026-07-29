import { redirect } from 'next/navigation';

import { BrandMark } from '../../components/brand-mark';
import { SignInForm } from '../../components/sign-in-form';
import { getViewer } from '../../lib/session';
import {
  supabasePublishableKey,
  supabaseUrl,
} from '../../lib/supabase/config';

export const dynamic = 'force-dynamic';

const ERROR_MESSAGES: Record<string, string> = {
  no_account:
    'No workspace account matches that email. Ask your admin to invite you.',
  exchange_failed: 'That sign-in link was invalid or expired. Request a new one.',
  missing_code: 'That sign-in link was incomplete. Request a new one.',
};

export default async function LoginPage({
  searchParams,
}: {
  searchParams: Promise<{ error?: string }>;
}) {
  const viewer = await getViewer();
  if (viewer) {
    redirect('/drift');
  }
  const { error } = await searchParams;
  const message = error ? (ERROR_MESSAGES[error] ?? null) : null;

  return (
    <div className="mx-auto flex min-h-screen max-w-md flex-col justify-center px-6">
      <div className="rounded-card border border-line bg-surface px-8 py-10">
        <span className="flex items-center gap-2.5">
          <BrandMark size={30} />
          <span className="eyebrow text-ink-muted">Groundtruth</span>
        </span>
        <h1 className="mt-4 font-display font-extrabold text-2xl text-ink">
          Sign in
        </h1>
        <p className="mt-2 text-sm text-ink-muted">
          Enter your work email to continue with single sign-on or a secure
          email link.
        </p>

        {message ? (
          <p
            role="alert"
            className="mt-4 rounded-card border border-line bg-raised px-4 py-3 text-sm text-ink"
          >
            {message}
          </p>
        ) : null}

        <SignInForm
          supabaseUrl={supabaseUrl()}
          supabasePublishableKey={supabasePublishableKey()}
        />
      </div>
    </div>
  );
}
