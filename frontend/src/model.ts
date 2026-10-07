export interface Geometry { x: number; y: number; width: number; height: number }
export interface Item extends Geometry {
  id: string;
  type: 'text' | 'image';
  text?: string;
  imageId?: string;
  mimeType?: string;
  createdAt: string;
  updatedAt: string;
  expiresAt: string;
  revision: number;
  size: number;
}
export interface Viewport { x: number; y: number; zoom: number; lastInteractionAt: number; updatedAt?: string }
export interface Board {
  items: Item[];
  viewport: Viewport;
  lastCreatedItemId: string;
  lastCreatedAt: string;
  memoryBytes: number;
  devices: number;
  limits: { maxItems: number; maxTextBytes: number; maxImageBytes: number; maxMemoryBytes: number };
  serverTime: string;
}
export interface BoardEvent { type: string; snapshot?: Board; item?: Item; id?: string; devices?: number; memoryBytes: number }
export const initialViewport: Viewport = { x: 0, y: 0, zoom: 1, lastInteractionAt: 0 };
export const clamp = (value: number, min: number, max: number) => Math.max(min, Math.min(max, value));
export const imageURL = (item: Item) => `/api/images/${item.imageId}`;

export class APIError extends Error {
  constructor(public status: number, message: string, public item?: Item) { super(message) }
}
const messages: Record<number, string> = {
  400: '入力内容を確認してください。',
  401: '認証の有効期限が切れました。共有トークンで接続してください。',
  403: '接続元またはアクセス方法が許可されていません。',
  404: 'カードが削除されたか、有効期限が切れています。',
  409: 'ほかの端末で変更されました。最新の位置を表示します。',
  413: 'サイズまたはキャンバスの上限を超えています。',
  415: 'PNG・JPEG・GIF画像、またはプレーンテキストを使ってください。',
  429: '操作が集中しています。少し待って再試行してください。',
};
export async function api<T>(path: string, init?: RequestInit): Promise<T> {
  const response = await fetch(path, { credentials: 'same-origin', cache: 'no-store', ...init });
  if (!response.ok) {
    const payload = await response.json().catch(() => ({}));
    throw new APIError(response.status, messages[response.status] ?? 'サーバーに接続できませんでした。', payload.item);
  }
  return response.status === 204 ? undefined as T : response.json();
}
export const jsonRequest = (method: string, value: unknown): RequestInit => ({ method, headers: { 'Content-Type': 'application/json' }, body: JSON.stringify(value) });

export function zoomAt(view: Viewport, zoom: number, point: { x: number; y: number }): Viewport {
  const next = clamp(zoom, .2, 3);
  return { ...view, zoom: next, x: point.x - (point.x - view.x) / view.zoom * next, y: point.y - (point.y - view.y) / view.zoom * next };
}

export async function clipboardImage(item: Item): Promise<Blob> {
  const response = await fetch(imageURL(item), { credentials: 'same-origin', cache: 'no-store' });
  if (!response.ok) throw new Error('画像の有効期限が切れた可能性があります。');
  const blob = await response.blob();
  if (blob.type === 'image/png') return blob;
  const bitmap = await createImageBitmap(blob);
  try {
    const canvas = document.createElement('canvas');
    canvas.width = bitmap.width;
    canvas.height = bitmap.height;
    canvas.getContext('2d')!.drawImage(bitmap, 0, 0);
    return await new Promise<Blob>((resolve, reject) => canvas.toBlob(value => value ? resolve(value) : reject(new Error('画像の変換に失敗しました。')), 'image/png'));
  } finally { bitmap.close() }
}

export async function copyItem(item: Item) {
  if (item.type === 'text') {
    if (!navigator.clipboard?.writeText) throw new Error('このブラウザではコピーできません。テキストを選択してコピーしてください。');
    await navigator.clipboard.writeText(item.text!);
  } else {
    if (!navigator.clipboard?.write || typeof ClipboardItem === 'undefined') throw new Error('画像コピーに対応していません。「開く」または「保存」を使ってください。');
    // 取得完了を待ってからwriteすると、Safariでユーザー操作の権限を失う。
    await navigator.clipboard.write([new ClipboardItem({ 'image/png': clipboardImage(item) })]);
  }
}
