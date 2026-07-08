import { redirect } from 'next/navigation';

import { oidcConfig } from '../../lib/oidc';
import { devModeEnabled, getViewer } from '../../lib/session';

export const dynamic = 'force-dynamic';

const ERROR_MESSAGES: Record<string, string> = {
  missing_flow: 'The sign-in attempt expired. Try again.',
  state_mismatch: 'The sign-in attempt could not be verified. Try again.',
  exchange_failed: 'Your identity provider rejected the sign-in. Try again.',
  no_account:
    'No workspace account matches that identity. Ask your admin to invite you.',
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
  const ssoConfigured = oidcConfig() !== null;
  const message = error ? ERROR_MESSAGES[error] : null;

  return (
    <div className="mx-auto flex min-h-[70vh] max-w-md flex-col justify-center">
      <div className="rounded-card border border-line bg-surface px-8 py-10">
        <p className="eyebrow text-ink-muted">Company Brain</p>
        <h1 className="mt-2 font-display font-extrabold text-2xl text-ink">
          Sign in
        </h1>
        <p className="mt-2 text-sm text-ink-muted">
          Access is provisioned by your workspace admin through your identity
          provider.
        </p>

        {message ? (
          <p className="mt-4 rounded-card border border-line bg-canvas px-4 py-3 text-sm text-ink">
            {message}
          </p>
        ) : null}

        {ssoConfigured ? (
          <a
            href="/api/auth/login"
            className="mt-6 block rounded-card bg-ink px-4 py-3 text-center text-sm font-medium text-surface"
          >
            Continue with SSO
          </a>
        ) : (
          <p className="mt-6 rounded-card border border-line bg-canvas px-4 py-3 text-sm text-ink-muted">
            Single sign-on is not configured. Set OIDC_ISSUER, OIDC_CLIENT_ID,
            OIDC_CLIENT_SECRET, and OIDC_REDIRECT_URI.
          </p>
        )}

        {devModeEnabled() ? (
          <p className="mt-4 text-xs text-ink-muted">
            Development mode is active, so requests without a session fall back
            to the seeded founder.
          </p>
        ) : null}
      </div>
    </div>
  );
}
