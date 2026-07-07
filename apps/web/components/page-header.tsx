export function PageHeader({
  title,
  subtitle,
}: {
  title: string;
  subtitle?: string;
}) {
  return (
    <header className="mb-8">
      <h1 className="font-display font-extrabold text-xl tracking-tight">
        {title}
      </h1>
      {subtitle ? (
        <p className="mt-1 text-sm text-ink-secondary">{subtitle}</p>
      ) : null}
    </header>
  );
}

export function EmptyState({
  title,
  detail,
}: {
  title: string;
  detail: string;
}) {
  return (
    <div className="rounded-card border border-line bg-surface px-6 py-10 text-center">
      <p className="text-md text-ink">{title}</p>
      <p className="mt-1 text-sm text-ink-secondary">{detail}</p>
    </div>
  );
}
