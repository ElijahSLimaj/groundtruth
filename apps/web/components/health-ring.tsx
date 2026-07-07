export function HealthRing({ score }: { score: number }) {
  const clamped = Math.max(0, Math.min(100, score));
  const radius = 16;
  const circumference = 2 * Math.PI * radius;
  const filled = (clamped / 100) * circumference;
  const color =
    clamped >= 75
      ? 'var(--color-verified)'
      : clamped >= 45
        ? 'var(--color-stale)'
        : 'var(--color-conflict)';

  return (
    <svg
      width="44"
      height="44"
      viewBox="0 0 44 44"
      role="img"
      aria-label={`Health ${Math.round(clamped)} of 100`}
    >
      <circle
        cx="22"
        cy="22"
        r={radius}
        fill="none"
        stroke="var(--color-line)"
        strokeWidth="3"
      />
      <circle
        cx="22"
        cy="22"
        r={radius}
        fill="none"
        stroke={color}
        strokeWidth="3"
        strokeLinecap="round"
        strokeDasharray={`${filled} ${circumference - filled}`}
        transform="rotate(-90 22 22)"
      />
      <text
        x="22"
        y="26"
        textAnchor="middle"
        fill="var(--color-ink-secondary)"
        style={{ font: '10px var(--font-jetbrains), monospace' }}
      >
        {Math.round(clamped)}
      </text>
    </svg>
  );
}
