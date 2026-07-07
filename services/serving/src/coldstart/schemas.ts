import { z } from 'zod';

export const ColdStartWire = z.object({
  entries: z
    .array(
      z.object({
        statement: z.string().min(1),
        domain: z.enum([
          'pricing',
          'policy',
          'process',
          'decision',
          'org',
          'positioning',
        ]),
        tier: z.enum(['bedrock', 'operational']),
        attributes_json: z.string(),
        confidence: z.number().min(0).max(1),
        source_chunk_indexes: z.array(z.number().int().min(0)),
        topic: z.string().min(1),
      }),
    )
    .max(10),
});
export type ColdStartWire = z.infer<typeof ColdStartWire>;

export function wordOverlap(a: string, b: string): number {
  const tokens = (s: string) =>
    new Set(
      s
        .toLowerCase()
        .split(/[^a-z0-9]+/)
        .filter((w) => w.length > 2),
    );
  const setA = tokens(a);
  const setB = tokens(b);
  if (setA.size === 0 || setB.size === 0) {
    return 0;
  }
  let shared = 0;
  for (const word of setA) {
    if (setB.has(word)) {
      shared++;
    }
  }
  return shared / Math.min(setA.size, setB.size);
}
