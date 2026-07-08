import type { Metadata } from 'next';
import { Archivo, Instrument_Sans, JetBrains_Mono } from 'next/font/google';
import Link from 'next/link';
import './globals.css';

import { getViewer } from '../lib/session';

const archivo = Archivo({
  variable: '--font-archivo',
  subsets: ['latin'],
  weight: ['600', '800'],
});

const instrument = Instrument_Sans({
  variable: '--font-instrument',
  subsets: ['latin'],
  weight: ['400', '500'],
});

const jetbrains = JetBrains_Mono({
  variable: '--font-jetbrains',
  subsets: ['latin'],
  weight: ['400', '500'],
});

export const metadata: Metadata = {
  title: 'Company Brain',
  description: 'Governed company knowledge with receipts',
};

const NAV = [
  { href: '/drift', label: 'Drift' },
  { href: '/queue', label: 'Queue' },
  { href: '/canon', label: 'Canon' },
  { href: '/connectors', label: 'Connectors' },
  { href: '/audit', label: 'Audit' },
];

export default async function RootLayout({
  children,
}: Readonly<{ children: React.ReactNode }>) {
  const viewer = await getViewer();
  return (
    <html
      lang="en"
      className={`${archivo.variable} ${instrument.variable} ${jetbrains.variable} h-full antialiased`}
    >
      <body className="min-h-full">
        <div className="flex min-h-screen">
          <nav
            aria-label="Primary"
            className="w-60 shrink-0 border-r border-line bg-surface px-4 py-6 flex flex-col gap-8"
          >
            <Link href="/drift" className="block px-2">
              <span className="font-display font-extrabold text-md tracking-tight">
                COMPANY BRAIN
              </span>
            </Link>
            <ul className="flex flex-col gap-1">
              {NAV.map((item) => (
                <li key={item.href}>
                  <Link
                    href={item.href}
                    className="block rounded-control px-3 py-2 text-sm text-ink-secondary hover:bg-raised hover:text-ink focus-visible:outline focus-visible:outline-2 focus-visible:outline-action"
                  >
                    {item.label}
                  </Link>
                </li>
              ))}
            </ul>
            <div className="mt-auto flex flex-col gap-3 px-3">
              {viewer ? (
                <form action="/api/auth/logout" method="post">
                  <p className="text-xs text-ink-secondary">
                    {viewer.displayName}
                  </p>
                  <button
                    type="submit"
                    className="mt-1 text-xs text-ink-muted underline hover:text-ink"
                  >
                    Sign out
                  </button>
                </form>
              ) : null}
              <p className="eyebrow text-ink-muted">truth has a receipt</p>
            </div>
          </nav>
          <main className="min-w-0 flex-1 px-10 py-8">{children}</main>
        </div>
      </body>
    </html>
  );
}
