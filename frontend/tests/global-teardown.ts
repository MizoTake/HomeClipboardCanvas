import { rm } from 'node:fs/promises';

export default async function teardown() { await rm('../tools/test-session.json', { force: true }) }
