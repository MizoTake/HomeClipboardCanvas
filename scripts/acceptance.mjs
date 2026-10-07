import { chromium } from '../frontend/node_modules/playwright/index.mjs';
import { spawn } from 'node:child_process';
import { mkdir, copyFile, writeFile, rm } from 'node:fs/promises';
import { once } from 'node:events';
import { createServer } from 'node:net';
import path from 'node:path';
import { fileURLToPath } from 'node:url';
import assert from 'node:assert/strict';

const root = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '..');
const sandbox = path.join(root, 'tools', 'acceptance');
if (!sandbox.startsWith(root + path.sep)) throw new Error('Acceptance directory is outside project');
await mkdir(sandbox, { recursive: true });
const exe = path.join(sandbox, 'HomeClipboardCanvas.exe');
await copyFile(path.join(root, 'dist', 'HomeClipboardCanvas.exe'), exe);
const config = path.join(sandbox, 'config.json');
const portReservation = createServer();
await new Promise(resolve => portReservation.listen(0, '127.0.0.1', resolve));
const port = portReservation.address().port;
await new Promise(resolve => portReservation.close(resolve));
const url = `http://127.0.0.1:${port}`;
let child;
let browser;
async function stop() {
  if (child && child.exitCode === null) { const exited = once(child, 'exit'); child.kill(); await exited }
}
async function start(ttlMinutes) {
  await writeFile(config, JSON.stringify({ ttlMinutes }), 'utf8');
  child = spawn(exe, ['serve', '--dev-http', '--bind', '127.0.0.1', '--port', String(port), '--config', config], { cwd: sandbox, windowsHide: true, stdio: ['ignore', 'pipe', 'pipe'] });
  let stderr = '';
  child.stderr.on('data', chunk => { stderr += chunk });
  child.stdout.resume();
  for (let i = 0; i < 100; i++) {
    if (child.exitCode !== null) throw new Error(`Isolated EXE exited: ${stderr}`);
    try { const response = await fetch(url + '/health'); if (response.status === 204) return } catch {}
    await new Promise(resolve => setTimeout(resolve, 100));
  }
  throw new Error('Isolated EXE did not start');
}
async function connect(page) {
  await page.goto(url);
  await page.getByText(/\d+ 台 接続中/).waitFor();
  assert.deepEqual(await page.context().cookies(), []);
}
async function create(page, text) {
  await page.getByRole('button', { name: 'テキストを置く' }).first().click();
  await page.getByLabel('共有するテキスト', { exact: true }).fill(text);
  await page.getByRole('button', { name: '共有する' }).click();
  await page.locator('.card-content').filter({ hasText: text }).waitFor();
}
const results = [];
try {
  await start(60);
  browser = await chromium.launch();
  const context = await browser.newContext();
  const page = await context.newPage();
  const errors = [];
  page.on('pageerror', error => errors.push(error.message));
  await connect(page);
  await create(page, '再起動後に残らないテキスト');
  results.push('単体EXEを隔離ディレクトリから起動し埋め込みUIでテキストを共有');
  const image = await page.evaluate(async () => {
    const canvas = document.createElement('canvas'); canvas.width = 48; canvas.height = 32;
    const ctx = canvas.getContext('2d'); ctx.fillStyle = '#357a53'; ctx.fillRect(0, 0, 48, 32);
    return canvas.toDataURL('image/png').split(',')[1];
  });
  await page.getByLabel('画像ファイル', { exact: true }).setInputFiles({ name: 'fixture.png', mimeType: 'image/png', buffer: Buffer.from(image, 'base64') });
  await page.locator('.card.image').waitFor();
  await page.getByText(/\d+ 台 接続中/).waitFor();
  await stop();
  await page.getByText('再接続中…', { exact: true }).waitFor();
  await start(1);
  await page.getByText(/\d+ 台 接続中/).waitFor();
  assert.equal(await page.locator('.card').count(), 0);
  assert.deepEqual(await page.context().cookies(), []);
  results.push('EXE終了・再起動後、ログインや再読み込みなしで自動再接続し空のキャンバスを表示');
  await create(page, '実時間TTLの対象');
  const started = Date.now();
  console.log('実時間TTLを検証中: 1分設定＋最大30秒の検出間隔');
  await page.locator('.card').waitFor({ state: 'detached', timeout: 100000 });
  const elapsed = Date.now() - started;
  assert.ok(elapsed >= 59000 && elapsed <= 95000, `Unexpected TTL timing: ${elapsed}ms`);
  assert.equal(await page.locator('.card').count(), 0);
  assert.deepEqual(errors, []);
  results.push(`実時間TTL: ${Math.round(elapsed / 1000)}秒後に期限切れカードがWebSocket通知で消去`);
  const summary = { checkedAt: new Date().toISOString(), results, browserErrors: errors, localhostOnly: true };
  await mkdir(path.join(root, 'artifacts'), { recursive: true });
  await writeFile(path.join(root, 'artifacts', 'acceptance.json'), JSON.stringify(summary, null, 2) + '\n', 'utf8');
  console.log(JSON.stringify(summary, null, 2));
} finally {
  await browser?.close();
  await stop();
  // 検証用設定だけを削除し、ビルド成果物の配布先には触れない。
  await rm(config, { force: true });
}
