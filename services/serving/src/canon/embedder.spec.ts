import { createServer } from 'node:http';
import type { AddressInfo, Server } from 'node:net';

import { VoyageEmbedder } from './embedder';

interface Captured {
  authorization?: string;
  body?: {
    model: string;
    input: string[];
    input_type: string;
  };
}

function serve(
  status: number,
  payload: unknown,
  captured: Captured,
): Promise<{ server: Server; url: string }> {
  return new Promise((resolve) => {
    const server = createServer((request, response) => {
      captured.authorization = request.headers.authorization;
      let raw = '';
      request.on('data', (chunk: Buffer) => {
        raw += chunk.toString();
      });
      request.on('end', () => {
        captured.body = JSON.parse(raw) as Captured['body'];
        response.writeHead(status, { 'content-type': 'application/json' });
        response.end(JSON.stringify(payload));
      });
    });
    server.listen(0, '127.0.0.1', () => {
      const { port } = server.address() as AddressInfo;
      resolve({ server, url: `http://127.0.0.1:${port}` });
    });
  });
}

describe('VoyageEmbedder', () => {
  it('embeds queries with the query input type', async () => {
    const captured: Captured = {};
    const { server, url } = await serve(
      200,
      { data: [{ embedding: [0.25, 0.75] }] },
      captured,
    );
    try {
      const embedder = new VoyageEmbedder('vk-test', 'voyage-large-2', 2, url);
      const vector = await embedder.embed('growth plan cost');
      expect(vector).toEqual([0.25, 0.75]);
      expect(captured.authorization).toBe('Bearer vk-test');
      expect(captured.body?.input_type).toBe('query');
      expect(captured.body?.input).toEqual(['growth plan cost']);
    } finally {
      server.close();
    }
  });

  it('rejects wrong dimensions', async () => {
    const captured: Captured = {};
    const { server, url } = await serve(
      200,
      { data: [{ embedding: [1] }] },
      captured,
    );
    try {
      const embedder = new VoyageEmbedder('vk-test', 'voyage-large-2', 2, url);
      await expect(embedder.embed('x')).rejects.toThrow('1 dimensions');
    } finally {
      server.close();
    }
  });

  it('surfaces api errors', async () => {
    const captured: Captured = {};
    const { server, url } = await serve(401, { detail: 'bad key' }, captured);
    try {
      const embedder = new VoyageEmbedder('vk-test', 'voyage-large-2', 2, url);
      await expect(embedder.embed('x')).rejects.toThrow('status 401');
    } finally {
      server.close();
    }
  });
});
