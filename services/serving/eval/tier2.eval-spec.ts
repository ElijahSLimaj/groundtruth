import fs from 'node:fs';
import path from 'node:path';

import { AnthropicLlmClient } from '../src/drift/llm';
import {
  TIER2_PROMPT_VERSION,
  TIER2_SYSTEM,
  tier2UserPrompt,
} from '../src/drift/prompts';
import { Tier2Result } from '../src/drift/schemas';

interface EvalCase {
  name: string;
  statement: string;
  attributes: Record<string, unknown>;
  chunk: string;
  source_type: string;
  expected: string[];
  expected_field?: string;
}

const apiKey = process.env.ANTHROPIC_API_KEY;
const model = process.env.DRIFT_TIER2_MODEL ?? 'claude-haiku-4-5';
const suite = apiKey ? describe : describe.skip;

const ACCURACY_FLOOR = 0.8;

suite('tier2 classification eval', () => {
  jest.setTimeout(300_000);

  it(`classifies at least ${ACCURACY_FLOOR * 100}% of cases correctly`, async () => {
    const cases = JSON.parse(
      fs.readFileSync(path.join(__dirname, 'tier2-cases.json'), 'utf8'),
    ) as EvalCase[];
    const client = new AnthropicLlmClient(apiKey as string);

    const failures: string[] = [];
    let fieldChecks = 0;
    let fieldHits = 0;

    for (const evalCase of cases) {
      const result = await client.completeJson(
        {
          model,
          system: TIER2_SYSTEM,
          user: tier2UserPrompt({
            statement: evalCase.statement,
            attributes: evalCase.attributes,
            chunkContent: evalCase.chunk,
            sourceType: evalCase.source_type,
          }),
          maxTokens: 256,
          promptVersion: TIER2_PROMPT_VERSION,
        },
        Tier2Result,
      );

      const pass = evalCase.expected.includes(result.relation);
      if (!pass) {
        failures.push(
          `${evalCase.name}: expected ${evalCase.expected.join('|')}, got ${result.relation} (${result.confidence})`,
        );
      }
      if (pass && evalCase.expected_field && result.relation !== 'unrelated') {
        fieldChecks++;
        if (result.conflicting_field === evalCase.expected_field) {
          fieldHits++;
        }
      }
    }

    const accuracy = (cases.length - failures.length) / cases.length;
    console.log(
      JSON.stringify(
        {
          model,
          prompt_version: TIER2_PROMPT_VERSION,
          cases: cases.length,
          accuracy,
          field_accuracy: fieldChecks > 0 ? fieldHits / fieldChecks : null,
          failures,
        },
        null,
        2,
      ),
    );
    expect(accuracy).toBeGreaterThanOrEqual(ACCURACY_FLOOR);
  });
});
