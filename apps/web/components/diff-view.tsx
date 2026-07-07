export function DiffView({
  current,
  proposed,
}: {
  current: string | null;
  proposed: string;
}) {
  return (
    <div className="flex flex-col gap-3">
      {current ? (
        <p className="text-md leading-relaxed text-ink-muted line-through decoration-conflict/60">
          {current}
        </p>
      ) : (
        <p className="eyebrow text-ink-muted">new entry</p>
      )}
      <p className="text-md leading-relaxed text-verified">{proposed}</p>
    </div>
  );
}

export function AttributesDiff({
  current,
  proposed,
}: {
  current: Record<string, unknown>;
  proposed: Record<string, unknown>;
}) {
  const keys = [
    ...new Set([...Object.keys(current), ...Object.keys(proposed)]),
  ].sort();
  if (keys.length === 0) {
    return null;
  }
  return (
    <dl className="font-mono text-xs bg-void rounded-control border border-line p-3 flex flex-col gap-1">
      {keys.map((key) => {
        const before = JSON.stringify(current[key]);
        const after = JSON.stringify(proposed[key]);
        const changed = before !== after;
        return (
          <div key={key} className="flex gap-2">
            <dt className="text-ink-muted min-w-40">{key}</dt>
            <dd className="flex gap-2 min-w-0">
              {changed && before !== undefined && (
                <span className="text-ink-muted line-through">{before}</span>
              )}
              <span className={changed ? 'text-verified' : 'text-ink-secondary'}>
                {after ?? before}
              </span>
            </dd>
          </div>
        );
      })}
    </dl>
  );
}
