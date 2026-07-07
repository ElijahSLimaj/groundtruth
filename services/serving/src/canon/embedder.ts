import { createHash } from 'node:crypto';

export interface Embedder {
  readonly modelId: string;
  embed(text: string): Promise<number[]>;
}

export const EMBEDDER = Symbol('EMBEDDER');

export class FakeEmbedder implements Embedder {
  constructor(
    readonly modelId = 'fake-embedder-v1',
    private readonly dims = 1536,
  ) {}

  embed(text: string): Promise<number[]> {
    const seed = createHash('sha256').update(text).digest();
    const vector = new Array<number>(this.dims);
    let norm = 0;
    for (let i = 0; i < this.dims; i++) {
      const counter = createHash('sha256')
        .update(Buffer.concat([seed, Buffer.from([i & 0xff, (i >> 8) & 0xff])]))
        .digest();
      const bits = counter.readUInt32BE(0);
      const v = Math.fround(Math.fround(bits % 2000) / 1000 - 1);
      vector[i] = v;
      norm += v * v;
    }
    norm = Math.sqrt(norm);
    if (norm === 0) {
      vector[0] = 1;
      return Promise.resolve(vector);
    }
    for (let i = 0; i < this.dims; i++) {
      vector[i] = Math.fround(vector[i] / norm);
    }
    return Promise.resolve(vector);
  }
}

export function vectorLiteral(vector: number[]): string {
  return '[' + vector.join(',') + ']';
}
