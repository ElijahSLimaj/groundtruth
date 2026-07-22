export function BrandMark({
  size = 28,
  className,
}: {
  size?: number;
  className?: string;
}) {
  return (
    <svg
      width={size}
      height={size}
      viewBox="0 0 32 32"
      fill="none"
      className={className}
      aria-hidden
      focusable="false"
    >
      <defs>
        <linearGradient id="brand-mark-tile" x1="0" y1="0" x2="0" y2="1">
          <stop offset="0" stopColor="#161b27" />
          <stop offset="1" stopColor="#0a0d14" />
        </linearGradient>
        <radialGradient id="brand-mark-spark" cx="0.5" cy="0.5" r="0.5">
          <stop offset="0" stopColor="#e8b04b" stopOpacity="0.55" />
          <stop offset="1" stopColor="#e8b04b" stopOpacity="0" />
        </radialGradient>
      </defs>
      <rect
        x="0.75"
        y="0.75"
        width="30.5"
        height="30.5"
        rx="8"
        fill="url(#brand-mark-tile)"
        stroke="#2a3342"
        strokeWidth="1"
      />
      <g
        stroke="#7b9ef8"
        strokeWidth="1.4"
        strokeOpacity="0.55"
        strokeLinecap="round"
      >
        <line x1="11.5" y1="11" x2="20.5" y2="9.5" />
        <line x1="11.5" y1="11" x2="17" y2="17.5" />
        <line x1="20.5" y1="9.5" x2="17" y2="17.5" />
        <line x1="9.5" y1="20" x2="17" y2="17.5" />
        <line x1="17" y1="17.5" x2="22.5" y2="21.5" />
        <line x1="9.5" y1="20" x2="11.5" y2="11" />
      </g>
      <circle cx="20.5" cy="9.5" r="5" fill="url(#brand-mark-spark)" />
      <g fill="#7b9ef8">
        <circle cx="11.5" cy="11" r="2" />
        <circle cx="9.5" cy="20" r="1.8" />
        <circle cx="17" cy="17.5" r="2.4" />
        <circle cx="22.5" cy="21.5" r="1.8" />
      </g>
      <circle cx="20.5" cy="9.5" r="2.4" fill="#e8b04b" />
    </svg>
  );
}
