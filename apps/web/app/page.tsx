import { BrainConstellation } from '../components/brain-constellation';
import { BrandMark } from '../components/brand-mark';
import { CAL_LINK, CAL_NAMESPACE, CalEmbed } from '../components/cal-embed';

function CtaButton({ label }: { label: string }) {
  return (
    <button
      type="button"
      data-cal-namespace={CAL_NAMESPACE}
      data-cal-link={CAL_LINK}
      data-cal-config='{"layout":"month_view"}'
      className="inline-block rounded-control bg-action px-6 py-3 text-sm font-medium text-void transition-opacity duration-150 hover:opacity-90 focus-visible:outline focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-action"
    >
      {label}
    </button>
  );
}

function SectionHeading({
  eyebrow,
  title,
}: {
  eyebrow: string;
  title: string;
}) {
  return (
    <div className="max-w-2xl">
      <p className="eyebrow text-ink-muted">{eyebrow}</p>
      <h2 className="mt-3 font-display text-xl font-extrabold tracking-tight text-ink">
        {title}
      </h2>
    </div>
  );
}

function ReceiptedAnswer() {
  return (
    <figure
      aria-label="Example answer with its receipt"
      className="rounded-card border border-line bg-surface p-6 shadow-panel"
    >
      <div className="flex items-center gap-2">
        <span className="inline-flex items-center gap-1.5 rounded-full border border-verified px-2.5 py-0.5 text-xs text-verified">
          <span aria-hidden>✓</span> Verified
        </span>
        <span className="font-mono text-xs text-ink-muted">
          verified 2026-06-30
        </span>
      </div>
      <blockquote className="mt-4 text-md text-ink">
        The Growth plan is 1,499 per month, billed annually. Discounts above 15
        percent require founder approval.
      </blockquote>
      <div className="mt-5 border-t border-line pt-4">
        <p className="eyebrow text-ink-muted">Receipt</p>
        <dl className="mt-3 grid gap-2 font-mono text-xs text-ink-secondary">
          <div className="flex justify-between gap-4">
            <dt>owner</dt>
            <dd className="text-ink">Ada Founder, pricing</dd>
          </div>
          <div className="flex justify-between gap-4">
            <dt>sources</dt>
            <dd className="text-ink">3 events · #sales · 2026-06-28</dd>
          </div>
          <div className="flex justify-between gap-4">
            <dt>version</dt>
            <dd className="text-ink">v4 · approved by 1 owner</dd>
          </div>
          <div className="flex justify-between gap-4">
            <dt>freshness</dt>
            <dd className="text-ink">verifies every 60 days</dd>
          </div>
        </dl>
      </div>
      <figcaption className="mt-4 text-xs text-ink-muted">
        Every answer carries this. To your team, and to every agent you deploy.
      </figcaption>
    </figure>
  );
}

const PAINS = [
  {
    title: 'The wiki lied',
    body: 'Your pricing changed in a Slack thread four months ago. The wiki still shows the old number, so every agent and every new hire repeats it with total confidence.',
  },
  {
    title: 'Search has no governance',
    body: 'Enterprise search returns a pile of embeddings. No owner, no approval, no way to tell a settled decision from a rejected idea somebody floated once.',
  },
  {
    title: 'Trust collapses once',
    body: 'One hallucinated policy in front of a customer and your team stops trusting every AI answer. The deployment stalls, and the tooling gets quietly abandoned.',
  },
];

const MECHANISMS = [
  {
    eyebrow: '01 · Receipts',
    title: 'Every answer is cited, owned, and dated',
    body: 'Answers carry exact trust labels: Verified, Stale, Stream signal, or No coverage. Each one resolves to source events, a named owner, an approval record, and a verified-at date. No coverage is a first class answer, never a guess.',
  },
  {
    eyebrow: '02 · Drift engine',
    title: 'When reality contradicts the docs, we catch it',
    body: 'The stream is compared against the canon continuously. A contradiction detected in 12 events becomes a drafted correction routed to the entry owner, who approves, edits, or rejects in one action. Stale entries decay visibly instead of lying quietly.',
  },
  {
    eyebrow: '03 · Agents over MCP',
    title: 'Your agents get the same governed truth',
    body: 'Any agent, any framework, any model vendor connects over the Model Context Protocol and queries the same canon with the same permissions as the human it acts for. Permissions are inherited at capture and enforced at query, never bolted on.',
  },
  {
    eyebrow: '04 · Cold start',
    title: 'Your canon, drafted from 90 days of history',
    body: 'Connect Slack, Gmail, Drive, and Notion. The system mines ninety days of history and drafts your initial canon: pricing, decisions, processes, org structure. You spend an afternoon approving instead of months writing.',
  },
];

const COMPARISON = [
  {
    dimension: 'Where answers come from',
    wiki: 'Pages someone wrote in 2023',
    search: 'Nearest embeddings, ranked',
    brain: 'Approved entries with provenance',
  },
  {
    dimension: 'When facts change',
    wiki: 'Someone remembers to edit',
    search: 'Old and new rank side by side',
    brain: 'Drift detected, owner approves the fix',
  },
  {
    dimension: 'Who is accountable',
    wiki: 'Nobody',
    search: 'Nobody',
    brain: 'A named owner per entry',
  },
  {
    dimension: 'What agents receive',
    wiki: 'Stale text to hallucinate from',
    search: 'Context with no trust signal',
    brain: 'Cited answers with trust labels',
  },
];

const SECURITY = [
  {
    title: 'Permissions are structural',
    body: 'ACLs are captured with every event and enforced on every query, for humans and agents alike. A private channel stays private by construction, not by filter.',
  },
  {
    title: 'Tenant isolation at the database',
    body: 'Row level security on every table, verified by an automated cross-tenant test suite on every deploy. A failed isolation test blocks the release unconditionally.',
  },
  {
    title: 'Encrypted at rest, erasable on request',
    body: 'Payloads are envelope-encrypted with per-tenant keys wrapped by a master key. Verified erasure requests tombstone, delete, and audit in one governed flow.',
  },
  {
    title: 'Everything is auditable',
    body: 'Every approval, rejection, merge, and erasure writes to an immutable audit log. That is what carries a security review, and what lets you defend the tool internally.',
  },
];

const TIERS = [
  {
    name: 'Core',
    price: '$500',
    detail: 'per month',
    features: [
      'Up to 50 employees',
      'Four connectors',
      'The full product, no crippled edition',
    ],
  },
  {
    name: 'Growth',
    price: '$1,500',
    detail: 'per month',
    features: ['Up to 300 employees', 'All connectors', 'Single sign-on'],
  },
  {
    name: 'Scale',
    price: 'from $4,000',
    detail: 'per month',
    features: [
      'Custom retention',
      'Audit exports',
      'Dedicated tenancy and compliance reporting',
    ],
  },
];

export default function LandingPage() {
  return (
    <div className="min-h-screen">
      <CalEmbed />
      <header className="sticky top-0 z-10 border-b border-line bg-void/90 backdrop-blur">
        <div className="mx-auto flex max-w-5xl items-center justify-between px-6 py-4">
          <span className="flex items-center gap-2.5">
            <BrandMark size={28} />
            <span className="font-display text-md font-extrabold tracking-tight">
              COMPANY BRAIN
            </span>
          </span>
          <nav aria-label="Landing" className="flex items-center gap-6">
            <a
              href="#pricing"
              className="text-sm text-ink-secondary hover:text-ink focus-visible:outline focus-visible:outline-2 focus-visible:outline-action"
            >
              Pricing
            </a>
            <button
              type="button"
              data-cal-namespace={CAL_NAMESPACE}
              data-cal-link={CAL_LINK}
              data-cal-config='{"layout":"month_view"}'
              className="rounded-control bg-action px-4 py-2 text-sm font-medium text-void transition-opacity duration-150 hover:opacity-90 focus-visible:outline focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-action"
            >
              Start a pilot
            </button>
          </nav>
        </div>
      </header>

      <main className="mx-auto max-w-5xl px-6">
        <section className="relative overflow-hidden py-20 md:py-28">
          <BrainConstellation className="pointer-events-none absolute left-1/2 top-1/2 hidden w-[1040px] max-w-none -translate-x-1/2 -translate-y-1/2 opacity-45 md:block" />
          <div className="relative z-10 grid items-center gap-12 md:grid-cols-2">
            <div>
              <p className="eyebrow text-ink-muted">
                Governed knowledge for companies deploying AI
              </p>
              <h1 className="mt-4 font-display text-2xl font-extrabold leading-tight tracking-tight text-ink">
                Your AI agents are confidently wrong about your company.
              </h1>
              <p className="mt-5 max-w-md text-md text-ink-secondary">
                Company Brain is the knowledge layer where every answer carries
                a receipt: source, owner, and freshness. The same governed canon
                for your team and your agents, with the same permissions.
              </p>
              <div className="mt-8 flex items-center gap-5">
                <CtaButton label="Start a 30-day pilot" />
                <span className="text-xs text-ink-muted">
                  Money back if the drafted canon is not useful.
                </span>
              </div>
            </div>
            <ReceiptedAnswer />
          </div>
        </section>

        <section aria-label="The wall" className="border-t border-line py-20">
          <SectionHeading
            eyebrow="The wall"
            title="Every agent deployment stalls the same way"
          />
          <p className="mt-4 max-w-2xl text-ink-secondary">
            The agent does not know the company. Deliberate knowledge rots in
            wikis nobody maintains, and the decisions that matter live in
            unsearchable chat history.
          </p>
          <div className="mt-10 grid gap-4 md:grid-cols-3">
            {PAINS.map((pain) => (
              <div
                key={pain.title}
                className="rounded-card border border-line bg-surface p-6"
              >
                <h3 className="font-display text-md font-extrabold text-ink">
                  {pain.title}
                </h3>
                <p className="mt-3 text-sm text-ink-secondary">{pain.body}</p>
              </div>
            ))}
          </div>
        </section>

        <section
          aria-label="How it works"
          className="border-t border-line py-20"
        >
          <SectionHeading
            eyebrow="How it works"
            title="A canon that stays true, not another pile of embeddings"
          />
          <div className="mt-10 grid gap-4 md:grid-cols-2">
            {MECHANISMS.map((mechanism) => (
              <div
                key={mechanism.eyebrow}
                className="rounded-card border border-line bg-surface p-6"
              >
                <p className="eyebrow text-action">{mechanism.eyebrow}</p>
                <h3 className="mt-3 font-display text-md font-extrabold text-ink">
                  {mechanism.title}
                </h3>
                <p className="mt-3 text-sm text-ink-secondary">
                  {mechanism.body}
                </p>
              </div>
            ))}
          </div>
          <div className="mt-8">
            <CtaButton label="Start a 30-day pilot" />
          </div>
        </section>

        <section
          aria-label="Why not a wiki or enterprise search"
          className="border-t border-line py-20"
        >
          <SectionHeading
            eyebrow="Why not a wiki or enterprise search"
            title="Governance is the difference, and it cannot be bolted on"
          />
          <div className="mt-10 overflow-x-auto">
            <table className="w-full min-w-[640px] border-collapse text-sm">
              <thead>
                <tr className="border-b border-line-strong text-left">
                  <th
                    scope="col"
                    className="py-3 pr-4 font-medium text-ink-muted"
                  />
                  <th
                    scope="col"
                    className="py-3 pr-4 font-medium text-ink-secondary"
                  >
                    Wiki
                  </th>
                  <th
                    scope="col"
                    className="py-3 pr-4 font-medium text-ink-secondary"
                  >
                    Enterprise search
                  </th>
                  <th scope="col" className="py-3 font-medium text-verified">
                    Company Brain
                  </th>
                </tr>
              </thead>
              <tbody>
                {COMPARISON.map((row) => (
                  <tr key={row.dimension} className="border-b border-line">
                    <th
                      scope="row"
                      className="py-4 pr-4 text-left font-medium text-ink"
                    >
                      {row.dimension}
                    </th>
                    <td className="py-4 pr-4 text-ink-muted">{row.wiki}</td>
                    <td className="py-4 pr-4 text-ink-muted">{row.search}</td>
                    <td className="py-4 text-ink">{row.brain}</td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        </section>

        <section aria-label="Security" className="border-t border-line py-20">
          <SectionHeading
            eyebrow="Security"
            title="Built to survive your security review"
          />
          <div className="mt-10 grid gap-4 md:grid-cols-2">
            {SECURITY.map((item) => (
              <div
                key={item.title}
                className="rounded-card border border-line bg-surface p-6"
              >
                <h3 className="font-display text-md font-extrabold text-ink">
                  {item.title}
                </h3>
                <p className="mt-3 text-sm text-ink-secondary">{item.body}</p>
              </div>
            ))}
          </div>
        </section>

        <section
          id="pricing"
          aria-label="Pricing"
          className="scroll-mt-24 border-t border-line py-20"
        >
          <SectionHeading
            eyebrow="Pricing"
            title="A platform fee, never per seat"
          />
          <p className="mt-4 max-w-2xl text-ink-secondary">
            Human access is free and unlimited, because human engagement keeps
            the canon alive. Agent queries are metered, with a generous included
            volume per tier. Your bill grows only as your agents do.
          </p>
          <div className="mt-10 grid gap-4 md:grid-cols-3">
            {TIERS.map((tier) => (
              <div
                key={tier.name}
                className="flex flex-col rounded-card border border-line bg-surface p-6"
              >
                <h3 className="font-display text-md font-extrabold text-ink">
                  {tier.name}
                </h3>
                <p className="mt-3">
                  <span className="font-mono text-xl text-ink">
                    {tier.price}
                  </span>{' '}
                  <span className="text-xs text-ink-muted">{tier.detail}</span>
                </p>
                <ul className="mt-4 flex flex-col gap-2 text-sm text-ink-secondary">
                  {tier.features.map((feature) => (
                    <li key={feature}>{feature}</li>
                  ))}
                </ul>
              </div>
            ))}
          </div>

          <div className="mt-8 rounded-card border border-line-strong bg-raised p-8">
            <p className="eyebrow text-verified">The pilot</p>
            <h3 className="mt-3 font-display text-lg font-extrabold text-ink">
              30 days, full product, cold start included
            </h3>
            <p className="mt-3 max-w-2xl text-sm text-ink-secondary">
              No free tier. Connect your tools, and within a day you review a
              drafted canon built from your last ninety days. Success is
              concrete: a reviewed, living canon your team and agents cite by
              day 30. If the drafted canon is not useful, you get your money
              back.
            </p>
            <div className="mt-6">
              <CtaButton label="Start a 30-day pilot" />
            </div>
          </div>
        </section>

        <section
          aria-label="Closing"
          className="border-t border-line py-20 text-center"
        >
          <p className="eyebrow text-ink-muted">Company Brain</p>
          <h2 className="mx-auto mt-4 max-w-xl font-display text-xl font-extrabold tracking-tight text-ink">
            Small and true beats big and comprehensive.
          </h2>
          <div className="mt-8">
            <CtaButton label="Start a 30-day pilot" />
          </div>
        </section>
      </main>

      <footer className="border-t border-line">
        <div className="mx-auto flex max-w-5xl items-center justify-between px-6 py-8">
          <span className="flex items-center gap-2.5">
            <BrandMark size={24} />
            <p className="eyebrow text-ink-muted">truth has a receipt</p>
          </span>
          <a
            href="#pricing"
            className="text-sm text-ink-secondary hover:text-ink focus-visible:outline focus-visible:outline-2 focus-visible:outline-action"
          >
            Pricing
          </a>
        </div>
      </footer>
    </div>
  );
}
