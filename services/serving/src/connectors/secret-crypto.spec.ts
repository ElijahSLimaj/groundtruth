import {
  gcmSeal,
  gcmOpen,
  masterKey,
  openConnectorSecret,
  sealConnectorSecret,
} from './secret-crypto';

const DATA_KEY = Buffer.alloc(32, 0x2a);
const CONNECTOR_ID = '11111111-1111-1111-1111-111111111111';

describe('secret-crypto envelope', () => {
  it('round-trips a raw payload through GCM', () => {
    const sealed = gcmSeal(DATA_KEY, Buffer.from('hello'));
    expect(gcmOpen(DATA_KEY, sealed).toString()).toBe('hello');
  });

  it('round-trips a connector secret bound to its connector id', () => {
    const secret = { access_token: 'a', refresh_token: 'r' };
    const b64 = sealConnectorSecret(DATA_KEY, CONNECTOR_ID, secret);
    expect(openConnectorSecret(DATA_KEY, CONNECTOR_ID, b64)).toEqual(secret);
  });

  it('opens the same golden vector the Go decoder opens', () => {
    const sealedB64 =
      '7skI2pAQfKmKSkV5jE+Ys+VzypKwtHuadg0c92MfS6mKWLD2DF7QuxiChVildAmrKckm3WvGJM7Zb8lSTq0zBrK/fL6Go/hpwIBU0/2vyy1BBcxJmqSdS6MD5W1ujFXpMA==';
    expect(openConnectorSecret(DATA_KEY, CONNECTOR_ID, sealedB64)).toEqual({
      access_token: 'ya29.demo-access',
      refresh_token: '1//demo-refresh',
    });
  });

  it('refuses a secret sealed for a different connector', () => {
    const b64 = sealConnectorSecret(DATA_KEY, CONNECTOR_ID, {
      access_token: 'a',
    });
    expect(() =>
      openConnectorSecret(
        DATA_KEY,
        '22222222-2222-2222-2222-222222222222',
        b64,
      ),
    ).toThrow();
  });

  it('rejects a master key that is not 32 bytes', () => {
    expect(() => masterKey('abcd')).toThrow();
  });
});
