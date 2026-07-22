'use client';

import { useState } from 'react';

import { createSupabaseBrowserClient } from '../lib/supabase/client';

type Status = 'idle' | 'sending' | 'sent' | 'error';

export function MagicLinkForm() {
  const [email, setEmail] = useState('');
  const [status, setStatus] = useState<Status>('idle');
  const [error, setError] = useState<string | null>(null);

  async function handleSubmit(event: React.FormEvent<HTMLFormElement>) {
    event.preventDefault();
    setStatus('sending');
    setError(null);
    const supabase = createSupabaseBrowserClient();
    const { error: otpError } = await supabase.auth.signInWithOtp({
      email,
      options: {
        emailRedirectTo: `${window.location.origin}/api/auth/callback`,
      },
    });
    if (otpError) {
      setStatus('error');
      setError(otpError.message);
      return;
    }
    setStatus('sent');
  }

  if (status === 'sent') {
    return (
      <div
        role="status"
        className="mt-6 rounded-card border border-line bg-raised px-4 py-4 text-sm text-ink"
      >
        <p className="font-medium">Check your email</p>
        <p className="mt-1 text-ink-secondary">
          We sent a sign-in link to {email}. Open it on this device to continue.
        </p>
        <button
          type="button"
          onClick={() => setStatus('idle')}
          className="mt-3 text-xs text-ink-muted underline hover:text-ink"
        >
          Use a different email
        </button>
      </div>
    );
  }

  return (
    <form onSubmit={handleSubmit} className="mt-6 flex flex-col gap-3">
      <label htmlFor="email" className="text-sm text-ink-secondary">
        Work email
      </label>
      <input
        id="email"
        type="email"
        name="email"
        autoComplete="email"
        required
        value={email}
        onChange={(event) => setEmail(event.target.value)}
        placeholder="you@company.com"
        className="rounded-control border border-line bg-void px-4 py-3 text-sm text-ink outline-none placeholder:text-ink-muted focus-visible:border-action focus-visible:outline focus-visible:outline-2 focus-visible:outline-offset-1 focus-visible:outline-action"
      />
      {status === 'error' && error ? (
        <p role="alert" className="text-sm text-conflict">
          {error}
        </p>
      ) : null}
      <button
        type="submit"
        disabled={status === 'sending'}
        className="rounded-control bg-action px-4 py-3 text-center text-sm font-medium text-void transition-opacity duration-150 hover:opacity-90 disabled:opacity-60 focus-visible:outline focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-action"
      >
        {status === 'sending' ? 'Sending…' : 'Send magic link'}
      </button>
    </form>
  );
}
