export interface SlackMessage {
  channel: string;
  text: string;
  blocks: unknown[];
}

export interface SlackWebApi {
  postMessage(message: SlackMessage): Promise<void>;
}

export const SLACK_WEB_API = Symbol('SLACK_WEB_API');

export class HttpSlackWebApi implements SlackWebApi {
  constructor(private readonly botToken: string) {}

  async postMessage(message: SlackMessage): Promise<void> {
    const response = await fetch('https://slack.com/api/chat.postMessage', {
      method: 'POST',
      headers: {
        Authorization: `Bearer ${this.botToken}`,
        'Content-Type': 'application/json',
      },
      body: JSON.stringify(message),
    });
    const body = (await response.json()) as { ok: boolean; error?: string };
    if (!body.ok) {
      throw new Error(`slack chat.postMessage failed: ${body.error}`);
    }
  }
}

export class DisabledSlackWebApi implements SlackWebApi {
  postMessage(): Promise<void> {
    return Promise.reject(new Error('slack app is not configured'));
  }
}
