import { request } from '@playwright/test';
import { mkdir } from 'node:fs/promises';

export default async function setup() {
  await mkdir('../tools', { recursive: true });
  const context = await request.newContext({ baseURL: 'http://127.0.0.1:18080' });
  try {
    const response = await context.post('/api/session', {
      headers: { Origin: 'http://127.0.0.1:18080' },
      data: { token: Buffer.alloc(32, 7).toString('base64url') },
    });
    if (response.status() !== 204) throw new Error('Test authentication failed');
    await context.storageState({ path: '../tools/test-session.json' });
  } finally { await context.dispose() }
}
