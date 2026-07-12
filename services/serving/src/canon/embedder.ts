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

export class VoyageEmbedder implements Embedder {
  constructor(
    private readonly apiKey: string,
    readonly modelId = 'voyage-large-2',
    private readonly dims = 1536,
    private readonly baseUrl = 'https://api.voyageai.com/v1',
  ) {}

  async embed(text: string): Promise<number[]> {
    const response = await fetch(`${this.baseUrl}/embeddings`, {
      method: 'POST',
      headers: {
        authorization: `Bearer ${this.apiKey}`,
        'content-type': 'application/json',
      },
      body: JSON.stringify({
        model: this.modelId,
        input: [text],
        input_type: 'query',
      }),
    });
    if (!response.ok) {
      const body = await response.text();
      throw new Error(
        `voyage embeddings: status ${response.status}: ${body.slice(0, 200)}`,
      );
    }
    const decoded = (await response.json()) as {
      data?: { embedding?: number[] }[];
    };
    const vector = decoded.data?.[0]?.embedding;
    if (!vector || vector.length !== this.dims) {
      throw new Error(
        `voyage returned ${vector?.length ?? 0} dimensions, expected ${this.dims}`,
      );
    }
    return vector;
  }
}

export function vectorLiteral(vector: number[]): string {
  return '[' + vector.join(',') + ']';
}
