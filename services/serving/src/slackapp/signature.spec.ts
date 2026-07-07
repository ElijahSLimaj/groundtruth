import { createHmac } from 'node:crypto';

import { verifySlackSignature } from './signature';

describe('verifySlackSignature', () => {
  const secret = 'test-signing-secret';
  const body = 'payload=%7B%22type%22%3A%22block_actions%22%7D';
  const now = 1_800_000_000;

  const sign = (timestamp: number) =>
    'v0=' +
    createHmac('sha256', secret)
      .update(`v0:${timestamp}:${body}`)
      .digest('hex');

  it('accepts a valid signature within tolerance', () => {
    expect(
      verifySlackSignature({
        signingSecret: secret,
        timestamp: String(now - 10),
        rawBody: body,
        signature: sign(now - 10),
        nowSeconds: now,
      }),
    ).toBe(true);
  });

  it('rejects a tampered body', () => {
    expect(
      verifySlackSignature({
        signingSecret: secret,
        timestamp: String(now),
        rawBody: body + 'tampered',
        signature: sign(now),
        nowSeconds: now,
      }),
    ).toBe(false);
  });

  it('rejects stale timestamps to stop replay', () => {
    expect(
      verifySlackSignature({
        signingSecret: secret,
        timestamp: String(now - 600),
        rawBody: body,
        signature: sign(now - 600),
        nowSeconds: now,
      }),
    ).toBe(false);
  });

  it('rejects garbage timestamps', () => {
    expect(
      verifySlackSignature({
        signingSecret: secret,
        timestamp: 'not-a-number',
        rawBody: body,
        signature: sign(now),
        nowSeconds: now,
      }),
    ).toBe(false);
  });
});
