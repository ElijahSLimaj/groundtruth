import { McpServer } from '@modelcontextprotocol/sdk/server/mcp.js';
import { StreamableHTTPServerTransport } from '@modelcontextprotocol/sdk/server/streamableHttp.js';
import { Controller, Post, Req, Res, UseGuards } from '@nestjs/common';
import type { Request, Response } from 'express';
import { z } from 'zod';

import { ApiKeyGuard } from '../auth/api-key.guard';
import { CurrentPrincipal } from '../auth/principal';
import type { Principal } from '../auth/principal';
import { CanonService } from '../canon/canon.service';

function asText(value: unknown): {
  content: { type: 'text'; text: string }[];
} {
  return { content: [{ type: 'text', text: JSON.stringify(value) }] };
}

export function buildMcpServer(
  canon: CanonService,
  principal: Principal,
): McpServer {
  const server = new McpServer({ name: 'company-brain', version: '1.0.0' });

  server.registerTool(
    'query',
    {
      description:
        'Ask the company brain a question. Returns an answer with trust label, citations that resolve to canon entries or stream events, open conflicts, and freshness. no_coverage is a first class answer.',
      inputSchema: {
        question: z.string().min(1),
        domains: z.array(z.string()).optional(),
        include_stream: z.boolean().optional(),
        max_citations: z.number().int().min(1).max(20).optional(),
      },
    },
    async (input) => asText(await canon.query(principal, input)),
  );

  server.registerTool(
    'get_entry',
    {
      description:
        'Fetch a canon entry with its attributes, version history, provenance links, and relations.',
      inputSchema: { entry_id: z.string().uuid() },
    },
    async (input) => asText(await canon.getEntry(principal, input.entry_id)),
  );

  server.registerTool(
    'list_conflicts',
    {
      description:
        'List open contradictions between the stream and the canon visible to the caller, optionally filtered by domain.',
      inputSchema: { domain: z.string().optional() },
    },
    async (input) => asText(await canon.listConflicts(principal, input.domain)),
  );

  server.registerTool(
    'propose_update',
    {
      description:
        'Submit a drift proposal into the owner approval pipeline. Provide entry_id to contradict an existing entry or domain to propose a new fact.',
      inputSchema: {
        entry_id: z.string().uuid().optional(),
        domain: z.string().optional(),
        statement: z.string().min(1),
        attributes: z.record(z.string(), z.unknown()).optional(),
      },
    },
    async (input) => asText(await canon.proposeUpdate(principal, input)),
  );

  return server;
}

@Controller('mcp')
@UseGuards(ApiKeyGuard)
export class McpController {
  constructor(private readonly canon: CanonService) {}

  @Post()
  async handle(
    @CurrentPrincipal() principal: Principal,
    @Req() request: Request,
    @Res() response: Response,
  ): Promise<void> {
    const server = buildMcpServer(this.canon, principal);
    const transport = new StreamableHTTPServerTransport({
      sessionIdGenerator: undefined,
    });
    response.on('close', () => {
      void transport.close();
      void server.close();
    });
    await server.connect(transport);
    await transport.handleRequest(request, response, request.body);
  }
}
