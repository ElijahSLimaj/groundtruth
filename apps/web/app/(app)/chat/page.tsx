import Link from 'next/link';

import { EmptyState, PageHeader } from '../../../components/page-header';
import { brainFetch, chatConfigured } from '../../../lib/brain-api';
import { requireViewer } from '../../../lib/session';
import { sendMessage } from './actions';

export const dynamic = 'force-dynamic';

interface ConversationSummary {
  id: string;
  title: string;
  updated_at: string;
}

interface Citation {
  entry_id?: string;
  trust?: string;
  statement?: string;
}

interface Message {
  id: string;
  role: string;
  content: string;
  citations: Citation[];
  created_at: string;
}

interface Artifact {
  id: string;
  kind: string;
  title: string;
  content: {
    sections?: { heading: string; body: string; entry_ids: string[] }[];
    slides?: { title: string; bullets: string[]; entry_ids: string[] }[];
  };
}

interface Conversation {
  id: string;
  title: string;
  messages: Message[];
  artifacts: Artifact[];
}

function Receipts({ citations }: { citations: Citation[] }) {
  const cited = citations.filter((c) => c.entry_id);
  if (cited.length === 0) {
    return null;
  }
  return (
    <details className="mt-3 border-t border-line pt-2">
      <summary className="eyebrow cursor-pointer text-ink-muted">
        Receipt · {cited.length} source{cited.length === 1 ? '' : 's'}
      </summary>
      <ul className="mt-2 flex flex-col gap-1 font-mono text-xs">
        {cited.map((citation, index) => (
          <li key={`${citation.entry_id}-${index}`}>
            <Link
              href={`/canon/${citation.entry_id}`}
              className="text-action hover:underline"
            >
              {citation.statement?.slice(0, 80) ?? citation.entry_id}
            </Link>
            {citation.trust ? (
              <span className="ml-2 text-ink-muted">{citation.trust}</span>
            ) : null}
          </li>
        ))}
      </ul>
    </details>
  );
}

function ArtifactCard({ artifact }: { artifact: Artifact }) {
  return (
    <div className="rounded-card border border-line bg-raised p-4">
      <p className="eyebrow text-verified">{artifact.kind}</p>
      <h3 className="mt-1 font-display text-md font-extrabold text-ink">
        {artifact.title}
      </h3>
      {artifact.content.slides?.map((slide, index) => (
        <div key={index} className="mt-3 border-t border-line pt-3">
          <p className="text-sm font-medium text-ink">{slide.title}</p>
          <ul className="mt-1 list-disc pl-5 text-sm text-ink-secondary">
            {slide.bullets.map((bullet, i) => (
              <li key={i}>{bullet}</li>
            ))}
          </ul>
          <p className="mt-1 font-mono text-xs text-ink-muted">
            cites {slide.entry_ids.length} entr
            {slide.entry_ids.length === 1 ? 'y' : 'ies'}
          </p>
        </div>
      ))}
      {artifact.content.sections?.map((section, index) => (
        <div key={index} className="mt-3 border-t border-line pt-3">
          <p className="text-sm font-medium text-ink">{section.heading}</p>
          <p className="mt-1 text-sm text-ink-secondary">{section.body}</p>
          <p className="mt-1 font-mono text-xs text-ink-muted">
            cites {section.entry_ids.length} entr
            {section.entry_ids.length === 1 ? 'y' : 'ies'}
          </p>
        </div>
      ))}
    </div>
  );
}

export default async function ChatPage({
  searchParams,
}: {
  searchParams: Promise<{ c?: string }>;
}) {
  const viewer = await requireViewer();
  const { c: selectedId } = await searchParams;

  if (!chatConfigured()) {
    return (
      <div className="max-w-3xl">
        <PageHeader title="Chat" subtitle="Talk to Groundtruth." />
        <EmptyState
          title="Chat is not configured"
          detail="Set SERVING_URL and INTERNAL_API_SECRET so the web app can reach the chat engine, and ANTHROPIC_API_KEY on the serving service to enable the model."
        />
      </div>
    );
  }

  const conversations = await brainFetch<ConversationSummary[]>(
    viewer,
    '/chat/conversations',
  );
  const conversation = selectedId
    ? await brainFetch<Conversation>(
        viewer,
        `/chat/conversations/${selectedId}`,
      )
    : null;

  return (
    <div className="flex h-full gap-6">
      <aside className="w-56 shrink-0">
        <PageHeader title="Chat" subtitle="Talk to your brain." />
        <Link
          href="/chat"
          className="block rounded-control border border-line px-3 py-2 text-sm text-ink hover:bg-raised focus-visible:outline focus-visible:outline-2 focus-visible:outline-action"
        >
          New conversation
        </Link>
        <ul className="mt-4 flex flex-col gap-1">
          {conversations.map((item) => (
            <li key={item.id}>
              <Link
                href={`/chat?c=${item.id}`}
                aria-current={item.id === selectedId ? 'page' : undefined}
                className={`block truncate rounded-control px-3 py-2 text-sm hover:bg-raised focus-visible:outline focus-visible:outline-2 focus-visible:outline-action ${
                  item.id === selectedId
                    ? 'bg-raised text-ink'
                    : 'text-ink-secondary'
                }`}
              >
                {item.title}
              </Link>
            </li>
          ))}
        </ul>
      </aside>

      <section className="flex min-w-0 flex-1 flex-col">
        <div className="flex-1">
          {!conversation ? (
            <EmptyState
              title="Ask Groundtruth"
              detail="Every answer is grounded in the canon and carries its receipt. Ask about pricing, policy, or process, or ask for a document or deck drafted from approved truth."
            />
          ) : (
            <ol className="flex flex-col gap-4">
              {conversation.messages.map((message) => (
                <li
                  key={message.id}
                  className={`max-w-2xl rounded-card border p-4 ${
                    message.role === 'user'
                      ? 'self-end border-line-strong bg-raised'
                      : 'self-start border-line bg-surface'
                  }`}
                >
                  <p className="eyebrow text-ink-muted">
                    {message.role === 'user' ? viewer.displayName : 'Brain'}
                  </p>
                  <p className="mt-2 whitespace-pre-wrap text-sm text-ink">
                    {message.content}
                  </p>
                  {message.role === 'assistant' ? (
                    <Receipts citations={message.citations} />
                  ) : null}
                </li>
              ))}
            </ol>
          )}
          {conversation && conversation.artifacts.length > 0 ? (
            <div className="mt-6 flex flex-col gap-3">
              <p className="eyebrow text-ink-muted">Artifacts</p>
              {conversation.artifacts.map((artifact) => (
                <ArtifactCard key={artifact.id} artifact={artifact} />
              ))}
            </div>
          ) : null}
        </div>

        <form action={sendMessage} className="mt-6 flex gap-2">
          <input
            type="hidden"
            name="conversation_id"
            value={conversation?.id ?? ''}
          />
          <label htmlFor="chat-input" className="sr-only">
            Message the brain
          </label>
          <input
            id="chat-input"
            name="content"
            required
            autoComplete="off"
            placeholder="Ask the brain, or ask it to draft a document or deck"
            className="min-w-0 flex-1 rounded-control border border-line bg-surface px-4 py-3 text-sm text-ink placeholder:text-ink-muted focus-visible:outline focus-visible:outline-2 focus-visible:outline-action"
          />
          <button
            type="submit"
            className="rounded-control bg-action px-5 py-3 text-sm font-medium text-void transition-opacity duration-150 hover:opacity-90 focus-visible:outline focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-action"
          >
            Send
          </button>
        </form>
      </section>
    </div>
  );
}
