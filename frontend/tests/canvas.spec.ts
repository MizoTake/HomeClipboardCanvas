import { test, expect, type Page, type WebSocketRoute } from '@playwright/test';
import path from 'node:path';
import { deflateSync } from 'node:zlib';
import { readFileSync } from 'node:fs';

const token = Buffer.alloc(32, 7).toString('base64url');
function makePNG() {
  const chunk = (type: string, data: Buffer) => {
    const input = Buffer.concat([Buffer.from(type), data]);
    let crc = 0xffffffff;
    for (const byte of input) { crc ^= byte; for (let i = 0; i < 8; i++) crc = (crc >>> 1) ^ (crc & 1 ? 0xedb88320 : 0) }
    const size = Buffer.alloc(4), checksum = Buffer.alloc(4);
    size.writeUInt32BE(data.length); checksum.writeUInt32BE((crc ^ 0xffffffff) >>> 0);
    return Buffer.concat([size, input, checksum]);
  };
  const header = Buffer.alloc(13); header.writeUInt32BE(32); header.writeUInt32BE(24, 4); header[8] = 8; header[9] = 6;
  const pixels = Buffer.alloc((32 * 4 + 1) * 24);
  for (let y = 0; y < 24; y++) for (let x = 0; x < 32; x++) { const i = y * 129 + 1 + x * 4; pixels[i] = 80 + x * 4; pixels[i + 1] = 120 + y * 4; pixels[i + 2] = 100; pixels[i + 3] = 255 }
  return Buffer.concat([Buffer.from([137, 80, 78, 71, 13, 10, 26, 10]), chunk('IHDR', header), chunk('IDAT', deflateSync(pixels)), chunk('IEND', Buffer.alloc(0))]);
}
const png = makePNG();

async function login(page: Page) {
  await page.context().addCookies(JSON.parse(readFileSync('../tools/test-session.json', 'utf8')).cookies);
  await page.goto('/');
  await expect(page.getByText(/\d+ 台 接続中/)).toBeVisible();
}
async function reset(page: Page) {
  await page.request.delete('/api/board', { headers: { Origin: 'http://127.0.0.1:18080' } });
  await page.evaluate(() => localStorage.clear());
  await expect(page.locator('.card')).toHaveCount(0);
  await page.getByRole('button', { name: '表示をリセット' }).click();
  await expect(page.getByRole('button', { name: '表示をリセット' })).toHaveText('100%');
}
async function addText(page: Page, text: string) {
  await page.getByRole('button', { name: 'テキストを置く' }).first().click();
  await page.getByLabel('共有するテキスト', { exact: true }).fill(text);
  await page.getByRole('button', { name: '共有する' }).click();
  await expect(page.locator('.card-content').filter({ hasText: text })).toBeVisible();
}

test.beforeEach(async ({ page }) => {
  await login(page); await reset(page);
  // テストの高速連続アクセスを理由に、本番のレート制限を無効化しない。
  await new Promise(resolve => setTimeout(resolve, 1000));
});

test('テキストを貼り付け、HTMLを解釈せずコピーできる', async ({ page }, info) => {
  const errors: string[] = [];
  page.on('pageerror', error => errors.push(error.message));
  await page.evaluate(() => {
    const data = new DataTransfer(); data.setData('text/plain', '日本語 <script>window.hacked=true</script>');
    document.dispatchEvent(new ClipboardEvent('paste', { clipboardData: data, bubbles: true }));
  });
  await expect(page.locator('.card-content')).toHaveText('日本語 <script>window.hacked=true</script>');
  expect(await page.evaluate(() => 'hacked' in window)).toBe(false);
  await page.evaluate(() => {
    Object.defineProperty(navigator, 'clipboard', { configurable: true, value: { writeText: async (value: string) => { (window as unknown as Record<string, string>).copied = value } } });
  });
  await page.getByRole('button', { name: 'テキストをコピー' }).click();
  expect(await page.evaluate(() => (window as unknown as Record<string, string>).copied)).toContain('日本語');
  expect(errors).toEqual([]);
  await page.screenshot({ path: path.join('..', 'artifacts', `${info.project.name}-text.png`), fullPage: true });
});

test('画像優先の貼り付けと元画像の取得・保存・コピー', async ({ page }) => {
  await page.evaluate(async data => {
    const bytes = Uint8Array.from(atob(data), c => c.charCodeAt(0));
    const transfer = new DataTransfer();
    transfer.items.add(new File([bytes], 'capture.png', { type: 'image/png' }));
    transfer.setData('text/plain', '画像より優先されないテキスト');
    document.dispatchEvent(new ClipboardEvent('paste', { clipboardData: transfer, bubbles: true }));
  }, png.toString('base64'));
  await expect(page.locator('.card.image')).toHaveCount(1);
  await expect(page.locator('.card.text')).toHaveCount(0);
  const src = await page.getByAltText('共有された画像').getAttribute('src');
  const response = await page.request.get(src!);
  expect((await response.body()).equals(png)).toBe(true);
  await expect(page.getByRole('link', { name: '開く' })).toHaveAttribute('target', '_blank');
  await expect(page.getByRole('link', { name: '保存' })).toHaveAttribute('download', /\.png$/);
  await page.evaluate(() => {
    Object.defineProperty(navigator, 'clipboard', { configurable: true, value: { write: async (items: ClipboardItem[]) => { const blob = await items[0].getType('image/png'); (window as unknown as Record<string, number>).copiedBytes = blob.size } } });
  });
  await page.getByRole('button', { name: '画像をコピー' }).click();
  await expect.poll(() => page.evaluate(() => (window as unknown as Record<string, number>).copiedBytes)).toBe(png.length);
});

test('2つのブラウザで追加・移動・削除・全削除が同期する', async ({ page, browser }) => {
  const second = await browser.newPage();
  await login(second);
  await addText(page, 'リアルタイム共有');
  await expect(second.locator('.card-content')).toHaveText('リアルタイム共有');
  await page.getByRole('button', { name: 'カードを削除' }).click();
  await expect(second.locator('.card')).toHaveCount(0);
  await addText(page, '全削除の対象');
  await page.getByRole('button', { name: 'すべて削除', exact: true }).first().click();
  await page.getByRole('dialog').getByRole('button', { name: 'すべて削除', exact: true }).click();
  await expect(second.locator('.card')).toHaveCount(0);
  await second.close();
});

test('カード移動・サイズ変更・パン・ズームが保存される', async ({ page }, info) => {
  test.skip(info.project.name === 'mobile', 'モバイルは専用のタッチ操作テストで検証');
  await addText(page, '位置とサイズ');
  const card = page.locator('.card');
  const initial = await card.boundingBox();
  const grip = await page.locator('.card-grip').boundingBox();
  await page.mouse.move(grip!.x + 70, grip!.y + 15); await page.mouse.down(); await page.mouse.move(grip!.x + 170, grip!.y + 75, { steps: 8 }); await page.mouse.up();
  await expect.poll(async () => (await card.boundingBox())!.x).toBeCloseTo(initial!.x + 100, 0);
  await expect.poll(async () => (await (await page.request.get('/api/board')).json()).items[0].x).toBeCloseTo(initial!.x + 100, 0);
  const resize = await page.getByRole('button', { name: 'カードのサイズ変更' }).boundingBox();
  await page.mouse.move(resize!.x + 7, resize!.y + 7); await page.mouse.down(); await page.mouse.move(resize!.x + 87, resize!.y + 47, { steps: 8 }); await page.mouse.up();
  await expect.poll(async () => (await card.boundingBox())!.width).toBeCloseTo(initial!.width + 80, 0);
  const canvas = await page.getByTestId('canvas').boundingBox();
  await page.mouse.move(canvas!.x + 25, canvas!.y + 25); await page.mouse.down(); await page.mouse.move(canvas!.x + 85, canvas!.y + 55, { steps: 8 }); await page.mouse.up();
  await expect(page.getByTestId('canvas-plane')).toHaveAttribute('style', /translate\(60px, 30px\)/);
  await page.getByRole('button', { name: '拡大', exact: true }).click();
  await expect(page.getByRole('button', { name: '表示をリセット' })).toHaveText('120%');
  await expect.poll(async () => (await (await page.request.get('/api/board')).json()).viewport.zoom).toBe(1.2);
});

test('再接続で最新の完全状態を取得し表示位置は勝手に動かない', async ({ page, context }) => {
  await addText(page, '再接続のカード');
  await page.getByRole('button', { name: '拡大', exact: true }).click();
  await context.setOffline(true);
  await expect(page.getByText('再接続中…')).toBeVisible({ timeout: 35000 });
  await context.setOffline(false);
  await expect(page.getByText(/\d+ 台 接続中/)).toBeVisible({ timeout: 20000 });
  await expect(page.locator('.card-content')).toHaveText('再接続のカード');
  await expect(page.getByRole('button', { name: '表示をリセット' })).toHaveText('120%');
});

test('共有URLからトークンを消し、別端末の表示操作に追従しない', async ({ page, browser }, info) => {
  const second = await browser.newPage();
  await second.goto(`/?token=${token}`);
  await expect(second.getByText(/\d+ 台 接続中/)).toBeVisible();
  expect(second.url()).not.toContain('token');
  await second.getByRole('button', { name: '拡大', exact: true }).click();
  await expect(page.getByRole('button', { name: '表示をリセット' })).toHaveText('100%');
  await second.close();
  await page.screenshot({ path: path.join('..', 'artifacts', `${info.project.name}-empty.png`), fullPage: true });
});

test('実際のブラウザClipboard APIでテキストと画像をコピーできる', async ({ page, context }) => {
  await context.grantPermissions(['clipboard-read', 'clipboard-write']);
  await addText(page, '実際のクリップボード検証');
  await page.getByRole('button', { name: 'テキストをコピー' }).click();
  expect(await page.evaluate(() => navigator.clipboard.readText())).toBe('実際のクリップボード検証');
  await page.evaluate(() => navigator.clipboard.writeText('Ctrl+Vの検証'));
  await page.getByTestId('canvas').click({ position: { x: 15, y: 15 } });
  await page.keyboard.press('Control+V');
  await expect(page.locator('.card-content').filter({ hasText: 'Ctrl+Vの検証' })).toBeVisible();
  await page.getByLabel('画像ファイル', { exact: true }).setInputFiles({ name: 'test.png', mimeType: 'image/png', buffer: png });
  await expect(page.locator('.card.image')).toHaveCount(1);
  await page.getByRole('button', { name: '画像をコピー' }).click();
  await expect(page.getByRole('status').filter({ hasText: 'クリップボードにコピーしました。' })).toBeVisible();
  await expect.poll(() => page.evaluate(async () => (await navigator.clipboard.read()).some(item => item.types.includes('image/png')))).toBe(true);
});

test('画像コピー非対応時は開く・保存を案内する', async ({ page }) => {
  await page.getByLabel('画像ファイル', { exact: true }).setInputFiles({ name: 'test.png', mimeType: 'image/png', buffer: png });
  await expect(page.locator('.card.image')).toHaveCount(1);
  await page.evaluate(() => Object.defineProperty(navigator, 'clipboard', { value: undefined, configurable: true }));
  await page.getByRole('button', { name: '画像をコピー' }).click();
  await expect(page.getByText('画像コピーに対応していません。「開く」または「保存」を使ってください。')).toBeVisible();
  await expect(page.getByRole('link', { name: '開く' })).toBeVisible();
  await expect(page.getByRole('link', { name: '保存' })).toBeVisible();
});

test('期限切れ通知を受け取るとカードが消える', async ({ page }) => {
  await addText(page, '期限切れ対象');
  const id = await page.locator('.card').getAttribute('data-item-id');
  let route: WebSocketRoute | undefined;
  await page.routeWebSocket('**/api/ws', ws => { route = ws; ws.connectToServer() });
  await page.reload();
  await expect(page.getByText(/\d+ 台 接続中/)).toBeVisible();
  await expect(page.locator('.card')).toHaveCount(1);
  await expect.poll(() => !!route).toBe(true);
  route!.send(JSON.stringify({ type: 'item.expired', id, memoryBytes: 0 }));
  await expect(page.locator('.card')).toHaveCount(0);
});

test('スマートフォンで移動・サイズ変更・2本指操作・長押しが使える', async ({ page, context }, info) => {
  test.skip(info.project.name !== 'mobile', 'タッチ操作はモバイルプロジェクトで検証');
  await addText(page, 'タッチ操作');
  const client = await context.newCDPSession(page);
  const initial = await page.locator('.card').boundingBox();
  const handle = await page.locator('.card-grip').boundingBox();
  await client.send('Input.dispatchTouchEvent', { type: 'touchStart', touchPoints: [{ x: handle!.x + 60, y: handle!.y + 15, id: 1 }] });
  await client.send('Input.dispatchTouchEvent', { type: 'touchMove', touchPoints: [{ x: handle!.x + 40, y: handle!.y + 35, id: 1 }] });
  await client.send('Input.dispatchTouchEvent', { type: 'touchEnd', touchPoints: [] });
  await expect.poll(async () => (await page.locator('.card').boundingBox())!.x).toBeCloseTo(initial!.x - 20, 0);
  const resize = await page.getByRole('button', { name: 'カードのサイズ変更' }).boundingBox();
  await client.send('Input.dispatchTouchEvent', { type: 'touchStart', touchPoints: [{ x: resize!.x + 5, y: resize!.y + 5, id: 1 }] });
  await client.send('Input.dispatchTouchEvent', { type: 'touchMove', touchPoints: [{ x: resize!.x + 25, y: resize!.y + 25, id: 1 }] });
  await client.send('Input.dispatchTouchEvent', { type: 'touchEnd', touchPoints: [] });
  await expect.poll(async () => (await page.locator('.card').boundingBox())!.width).toBeCloseTo(initial!.width + 20, 0);
  const canvas = await page.getByTestId('canvas').boundingBox();
  const y = canvas!.y + 65;
  await client.send('Input.dispatchTouchEvent', { type: 'touchStart', touchPoints: [{ x: 60, y, id: 1 }, { x: 160, y, id: 2 }] });
  await client.send('Input.dispatchTouchEvent', { type: 'touchMove', touchPoints: [{ x: 80, y: y + 30, id: 1 }, { x: 230, y: y + 30, id: 2 }] });
  await client.send('Input.dispatchTouchEvent', { type: 'touchEnd', touchPoints: [] });
  await expect(page.getByRole('button', { name: '表示をリセット' })).toHaveText('150%');
  await page.getByRole('button', { name: '表示をリセット' }).click();
  const grip = await page.locator('.card-grip').boundingBox();
  await client.send('Input.dispatchTouchEvent', { type: 'touchStart', touchPoints: [{ x: grip!.x + 80, y: grip!.y + 15, id: 1 }] });
  await expect(page.getByRole('menu')).toBeVisible();
  await client.send('Input.dispatchTouchEvent', { type: 'touchEnd', touchPoints: [] });
  await expect(page.getByRole('menuitem', { name: 'コピー' })).toBeVisible();
  await client.detach();
});

test('キーボードでカード位置とサイズを変更しDeleteで削除できる', async ({ page }) => {
  await addText(page, 'キーボード操作');
  const card = page.locator('.card');
  const before = await card.boundingBox();
  await card.focus(); await page.keyboard.press('ArrowRight');
  await expect.poll(async () => (await card.boundingBox())!.x).toBeCloseTo(before!.x + 20, 0);
  await page.getByRole('button', { name: 'カードのサイズ変更' }).focus(); await page.keyboard.press('ArrowRight');
  await expect.poll(async () => (await card.boundingBox())!.width).toBeCloseTo(before!.width + 20, 0);
  const deleted = page.waitForResponse(response => response.request().method() === 'DELETE' && response.url().includes('/api/items/'));
  await page.keyboard.press('Delete');
  expect((await deleted).status()).toBe(204);
  await expect(card).toHaveCount(0);
});

test('変更の応答が削除より遅れて届いてもカードが復活しない', async ({ page }) => {
  await addText(page, '遅い応答の対象');
  let release!: () => void;
  let ready!: () => void;
  const held = new Promise<void>(resolve => { release = resolve });
  const forwarded = new Promise<void>(resolve => { ready = resolve });
  await page.route('**/api/items/*', async route => {
    if (route.request().method() !== 'PATCH') { await route.continue(); return }
    const response = await route.fetch(); ready(); await held; await route.fulfill({ response });
  });
  await page.locator('.card').focus(); await page.keyboard.press('ArrowRight');
  await forwarded;
  await page.getByRole('button', { name: 'カードを削除' }).click();
  await expect(page.locator('.card')).toHaveCount(0);
  release();
  await page.waitForLoadState('networkidle');
  await expect(page.locator('.card')).toHaveCount(0);
});
