interface Node {
  x: number;
  y: number;
  tone: 'verified' | 'action' | 'stream' | 'positive' | 'dim';
  r: number;
  halo?: boolean;
}

const TONE: Record<Node['tone'], string> = {
  verified: 'var(--color-verified)',
  action: 'var(--color-action)',
  stream: 'var(--color-stream)',
  positive: 'var(--color-positive)',
  dim: 'var(--color-ink-muted)',
};

const NODES: Node[] = [
  { x: 150, y: 42, tone: 'verified', r: 3 },
  { x: 110, y: 74, tone: 'action', r: 3 },
  { x: 92, y: 122, tone: 'stream', r: 2.5 },
  { x: 122, y: 168, tone: 'action', r: 3 },
  { x: 172, y: 84, tone: 'action', r: 3 },
  { x: 152, y: 128, tone: 'stream', r: 2.5 },
  { x: 188, y: 172, tone: 'action', r: 3 },
  { x: 82, y: 170, tone: 'dim', r: 2 },
  { x: 132, y: 214, tone: 'stream', r: 2.5 },
  { x: 220, y: 54, tone: 'action', r: 3 },
  { x: 220, y: 120, tone: 'verified', r: 3.5 },
  { x: 220, y: 186, tone: 'action', r: 3 },
  { x: 220, y: 236, tone: 'stream', r: 2.5 },
  { x: 290, y: 42, tone: 'verified', r: 3 },
  { x: 330, y: 74, tone: 'action', r: 3 },
  { x: 348, y: 122, tone: 'stream', r: 2.5 },
  { x: 318, y: 168, tone: 'action', r: 3 },
  { x: 268, y: 84, tone: 'action', r: 3 },
  { x: 288, y: 128, tone: 'positive', r: 3 },
  { x: 252, y: 172, tone: 'action', r: 3 },
  { x: 358, y: 170, tone: 'dim', r: 2 },
  { x: 308, y: 214, tone: 'stream', r: 2.5 },
];

const EDGES: [number, number][] = [
  [1, 2],
  [2, 3],
  [3, 4],
  [4, 9],
  [1, 5],
  [5, 6],
  [6, 7],
  [4, 8],
  [8, 9],
  [5, 10],
  [6, 11],
  [7, 12],
  [9, 13],
  [2, 5],
  [14, 15],
  [15, 16],
  [16, 17],
  [17, 22],
  [14, 18],
  [18, 19],
  [19, 20],
  [17, 21],
  [21, 22],
  [18, 10],
  [19, 11],
  [20, 12],
  [22, 13],
  [15, 18],
  [10, 11],
  [11, 12],
  [12, 13],
  [10, 14],
];

const SIGNALS: [number, number, number][] = [
  [1, 14, 0],
  [6, 19, 1.6],
  [5, 7, 2.3],
  [18, 20, 0.7],
  [3, 12, 1.2],
];

const at = (id: number) => NODES[id - 1];

export function BrainConstellation({ className }: { className?: string }) {
  return (
    <svg
      viewBox="0 0 440 280"
      preserveAspectRatio="xMidYMid meet"
      className={className}
      role="img"
      aria-label="A neural network of two hemispheres, its nodes carrying the trust colors of the canon"
    >
      <defs>
        <radialGradient id="brain-core" cx="50%" cy="45%" r="60%">
          <stop offset="0%" stopColor="var(--color-action)" stopOpacity="0.18" />
          <stop
            offset="50%"
            stopColor="var(--color-verified)"
            stopOpacity="0.06"
          />
          <stop offset="100%" stopColor="var(--color-void)" stopOpacity="0" />
        </radialGradient>
      </defs>

      <ellipse cx="220" cy="135" rx="215" ry="150" fill="url(#brain-core)" />

      <g className="brain-float">
        {EDGES.map(([from, to], index) => {
          const a = at(from);
          const b = at(to);
          return (
            <line
              key={index}
              x1={a.x}
              y1={a.y}
              x2={b.x}
              y2={b.y}
              stroke="var(--color-action)"
              strokeWidth="1"
              strokeOpacity="0.2"
            />
          );
        })}

        {SIGNALS.map(([from, to, delay], index) => {
          const a = at(from);
          const b = at(to);
          return (
            <line
              key={index}
              x1={a.x}
              y1={a.y}
              x2={b.x}
              y2={b.y}
              pathLength={100}
              className="brain-signal"
              style={{ animationDelay: `${delay}s` }}
              stroke="var(--color-verified)"
              strokeWidth="1.1"
              strokeOpacity="0.5"
              strokeLinecap="round"
            />
          );
        })}

        {NODES.map((node, index) => (
          <g key={index}>
            {node.halo ? (
              <circle
                cx={node.x}
                cy={node.y}
                r={node.r * 3}
                fill={TONE[node.tone]}
                className="brain-halo"
                style={{ animationDelay: `${(index % 6) * 0.5}s` }}
              />
            ) : null}
            <circle
              cx={node.x}
              cy={node.y}
              r={node.r}
              fill={TONE[node.tone]}
              className="brain-node"
              style={{ animationDelay: `${(index % 5) * 0.7}s` }}
            />
          </g>
        ))}
      </g>
    </svg>
  );
}
