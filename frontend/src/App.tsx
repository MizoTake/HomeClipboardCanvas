import { useCallback, useEffect, useRef, useState, type FormEvent } from 'react';
import { Canvas } from './Canvas';
import { api, APIError, copyItem, initialViewport, jsonRequest, zoomAt, type Geometry, type Item, type Viewport } from './model';
import { useBoard } from './useBoard';

export default function App() {
  const [notice, setNotice] = useState('');
  const noticeTimer = useRef<ReturnType<typeof setTimeout> | undefined>(undefined);
  const notify = useCallback((message: string) => { setNotice(message); clearTimeout(noticeTimer.current); noticeTimer.current = setTimeout(() => setNotice(''), 6500) }, []);
  const { board, authenticated, checking, online, initial, login, clockOffset, resetAuth } = useBoard(notify);
  const [token, setToken] = useState('');
  const [loggingIn, setLoggingIn] = useState(false);
  const [viewport, setViewport] = useState<Viewport>(initialViewport);
  const viewportRef = useRef(viewport);
  viewportRef.current = viewport;
  const [selected, setSelected] = useState<string | null>(null);
  const [compose, setCompose] = useState(false);
  const [text, setText] = useState('');
  const [clear, setClear] = useState(false);
  const [help, setHelp] = useState(false);
  const [busy, setBusy] = useState(false);
  const [now, setNow] = useState(Date.now());
  const [seen, setSeen] = useState(() => { try { return Number(localStorage.getItem('hcc.lastVisit') ?? 0) } catch { return 0 } });
  const main = useRef<HTMLElement>(null);
  const fileInput = useRef<HTMLInputElement>(null);
  const placement = useRef(0);

  useEffect(() => { if (initial) { setViewport({ ...initial.viewport, lastInteractionAt: 0 }); setSelected(null) } }, [initial]);
  useEffect(() => { const timer = setInterval(() => setNow(Date.now()), 10000); return () => { clearInterval(timer); clearTimeout(noticeTimer.current) } }, []);
  const onViewport = useCallback((v: Viewport) => { setViewport({ ...v, lastInteractionAt: Date.now() + clockOffset.current }) }, []);

  useEffect(() => {
    if (!authenticated || viewport.lastInteractionAt === 0) return;
    const timer = setTimeout(() => { void api('/api/viewport', jsonRequest('POST', viewport)).catch(() => {}) }, 800);
    return () => clearTimeout(timer);
  }, [viewport, authenticated]);
  useEffect(() => {
    if (!authenticated) return;
    const save = () => {
      const v = viewportRef.current;
      if (v.lastInteractionAt > 0) navigator.sendBeacon('/api/viewport', new Blob([JSON.stringify(v)], { type: 'application/json' }));
      try { localStorage.setItem('hcc.lastVisit', String(Date.now() + clockOffset.current)) } catch { /* ストレージ禁止でも共有操作を停止しない。 */ }
    };
    const visibility = () => { if (document.visibilityState === 'hidden') save() };
    addEventListener('pagehide', save); addEventListener('beforeunload', save); document.addEventListener('visibilitychange', visibility);
    return () => { removeEventListener('pagehide', save); removeEventListener('beforeunload', save); document.removeEventListener('visibilitychange', visibility) };
  }, [authenticated]);

  const handleError = (error: unknown) => {
    if (error instanceof APIError) {
      if (error.status === 401) resetAuth();
    }
    notify(error instanceof Error ? error.message : '操作に失敗しました。');
  };
  const geometry = (width = 340, height = 240): Geometry => {
    const rect = main.current!.getBoundingClientRect();
    const v = viewportRef.current;
    const offset = placement.current++ % 5 * 24;
    return { x: (rect.width / 2 - v.x) / v.zoom - width / 2 + offset, y: (rect.height / 2 - v.y) / v.zoom - height / 2 + offset, width, height };
  };
  const markSeen = (item: Item) => { const time = Date.parse(item.createdAt); setSeen(time); try { localStorage.setItem('hcc.lastVisit', String(time)) } catch { /* 私的ブラウジングでも操作を継続する。 */ } };
  const addText = async (value: string) => {
    if (!value.trim()) { notify('テキストを入力してください。'); return }
    if (!online) { notify('再接続が完了してから貼り付けてください。'); return }
    setBusy(true);
    try {
      const item = await api<Item>('/api/items', jsonRequest('POST', { text: value, ...geometry() }));
      setSelected(item.id); markSeen(item); setText(''); setCompose(false); notify('テキストを共有しました。');
    } catch (error) { handleError(error) } finally { setBusy(false) }
  };
  const addImage = async (blob: Blob) => {
    if (!online) { notify('再接続が完了してから貼り付けてください。'); return }
    if (!['image/png', 'image/jpeg', 'image/gif'].includes(blob.type)) { notify('PNG・JPEG・GIF画像を使ってください。SVGには対応していません。'); return }
    if (blob.size > board!.limits.maxImageBytes) { notify('画像サイズが上限を超えています。'); return }
    setBusy(true);
    try {
      const bitmap = await createImageBitmap(blob);
      if (bitmap.width * bitmap.height > 40000000) { bitmap.close(); throw new Error('画像の画素数が上限（4,000万画素）を超えています。') }
      const width = Math.max(260, Math.min(460, bitmap.width));
      const height = Math.max(200, Math.min(460, width * bitmap.height / bitmap.width + 98));
      bitmap.close();
      const query = new URLSearchParams(Object.entries(geometry(width, height)).map(([key, value]) => [key, String(value)]));
      const item = await api<Item>(`/api/images?${query}`, { method: 'POST', headers: { 'Content-Type': blob.type }, body: blob });
      setSelected(item.id); markSeen(item); notify('画像を共有しました。');
    } catch (error) { handleError(error) } finally { setBusy(false) }
  };
  const copy = async (item: Item) => { try { await copyItem(item); notify('クリップボードにコピーしました。') } catch (error) { handleError(error) } };
  const remove = async (item: Item) => { if (!online) { notify('再接続が完了してから操作してください。'); return }; try { await api(`/api/items/${item.id}`, { method: 'DELETE' }); setSelected(null) } catch (error) { handleError(error) } };
  // HTTP応答が削除通知より遅れて届いたときに、削除済みカードを復元しない。
  const update = async (item: Item, g: Geometry) => { if (!online) { notify('再接続が完了してから操作してください。'); return }; try { await api<Item>(`/api/items/${item.id}`, jsonRequest('PATCH', { x: g.x, y: g.y, width: g.width, height: g.height, revision: item.revision })) } catch (error) { handleError(error) } };

  useEffect(() => {
    if (!authenticated) return;
    const editable = (target: EventTarget | null) => target instanceof HTMLElement && !!target.closest('input,textarea,[contenteditable="true"]');
    const paste = (event: ClipboardEvent) => {
      if (editable(event.target) || compose || clear || help) return;
      const data = event.clipboardData;
      if (!data) return;
      event.preventDefault();
      const image = [...data.items].find(item => item.type.startsWith('image/'))?.getAsFile();
      if (image) void addImage(image);
      else { const value = data.getData('text/plain'); if (value) void addText(value) }
    };
    const keydown = (event: KeyboardEvent) => {
      if (editable(event.target) || compose || clear || help) return;
      const item = board?.items.find(value => value.id === selected);
      if (event.key === 'Escape') { setSelected(null); return }
      if (!item) return;
      if (event.key === 'Delete' || event.key === 'Backspace') { event.preventDefault(); void remove(item) }
      if ((event.ctrlKey || event.metaKey) && event.key.toLowerCase() === 'c' && !getSelection()?.toString()) { event.preventDefault(); void copy(item) }
    };
    addEventListener('paste', paste); addEventListener('keydown', keydown);
    return () => { removeEventListener('paste', paste); removeEventListener('keydown', keydown) };
  }, [authenticated, board, selected, online, compose, clear, help]);

  const pasteButton = async () => {
    if (!navigator.clipboard?.read) { setCompose(true); notify('入力欄に貼り付けるか、「画像を選ぶ」を使ってください。'); return }
    try {
      const contents = await navigator.clipboard.read();
      for (const content of contents) {
        const mime = content.types.find(type => type.startsWith('image/'));
        if (mime) { await addImage(await content.getType(mime)); return }
      }
      for (const content of contents) { if (content.types.includes('text/plain')) { await addText(await (await content.getType('text/plain')).text()); return } }
      notify('クリップボードにテキストや画像がありません。');
    } catch { notify('クリップボードの読み取りを許可するか、Ctrl＋Vを使ってください。スマートフォンでは入力欄への貼り付けも使えます。'); setCompose(true) }
  };
  const authenticate = async (event: FormEvent) => {
    event.preventDefault(); setLoggingIn(true);
    try { await login(token.trim()); setToken('') } catch (error) { handleError(error) } finally { setLoggingIn(false) }
  };
  const zoom = (factor: number) => { const rect = main.current!.getBoundingClientRect(); onViewport(zoomAt(viewport, viewport.zoom * factor, { x: rect.width / 2, y: rect.height / 2 })) };
  const latest = board?.items.find(item => item.id === board.lastCreatedItemId);
  const jump = () => {
    if (!latest) return;
    const rect = main.current!.getBoundingClientRect();
    const z = Math.min(1, (rect.width - 48) / latest.width, (rect.height - 100) / latest.height);
    onViewport({ x: rect.width / 2 - (latest.x + latest.width / 2) * z, y: rect.height / 2 - (latest.y + latest.height / 2) * z, zoom: z, lastInteractionAt: 0 });
    setSelected(latest.id); markSeen(latest);
  };
  const clearBoard = async () => { setBusy(true); try { await api('/api/board', { method: 'DELETE' }); setClear(false); setSelected(null); notify('すべてのカードを削除しました。') } catch (error) { handleError(error) } finally { setBusy(false) } };

  return <div className="app">
    <header className="app-header"><div className="brand"><span className="brand-symbol" aria-hidden="true">▤</span><div><h1>Home Clipboard <span>Canvas</span></h1><p>家のなかの、小さな受け渡し場所。</p></div></div>
      {authenticated && <div className={`connection ${online ? 'online' : ''}`} role="status"><span className="status-dot" />{online ? `${board?.devices ?? 0} 台 接続中` : '再接続中…'}</div>}
    </header>
    {!authenticated ? <main className="login-main"><div className="login-box"><p className="eyebrow">PRIVATE · LOCAL · TEMPORARY</p><h2>おうちのキャンバスに<br />つながろう。</h2><p>サーバーで発行した共有トークンを入力してください。<br />同じLANの端末だけがアクセスできます。</p>
      {checking ? <p role="status">接続を確認しています…</p> : <form onSubmit={authenticate}><label htmlFor="token">共有トークン</label><input id="token" type="password" value={token} onChange={e => setToken(e.target.value)} autoComplete="off" spellCheck={false} required placeholder="共有トークンを貼り付け" /><button className="primary" type="submit" disabled={loggingIn}>{loggingIn ? '接続しています…' : 'キャンバスを開く →'}</button></form>}
      <div className="privacy-note"><span aria-hidden="true">◷</span> データは一時的。サーバー終了時にすべて消えます。</div></div></main>
      : <>
        <nav className="toolbar" aria-label="キャンバス操作"><div className="add-tools"><button className="primary" onClick={() => setCompose(true)} disabled={!online}>＋ <span>テキストを置く</span></button><button onClick={() => void pasteButton()} disabled={!online}>▣ <span>貼り付け</span></button><button onClick={() => fileInput.current?.click()} disabled={!online}>▧ <span>画像を選ぶ</span></button></div><div className="board-tools"><span className="item-count">{board?.items.length ?? 0} / {board?.limits.maxItems ?? 100}</span><button className="clear-button" onClick={() => setClear(true)} disabled={!online || !board?.items.length}>すべて削除</button><button className="help-button" aria-label="操作ガイド" onClick={() => setHelp(true)}>?</button></div></nav>
        <main ref={main} className="board-area" onDragOver={e => e.preventDefault()} onDrop={e => { e.preventDefault(); const image = [...e.dataTransfer.files].find(file => file.type.startsWith('image/')); if (image) void addImage(image); else { const value = e.dataTransfer.getData('text/plain'); if (value) void addText(value) } }}>
          <Canvas items={board?.items ?? []} viewport={viewport} onViewport={onViewport} selected={selected} onSelect={setSelected} onUpdate={(item, g) => void update(item, g)} onCopy={item => void copy(item)} onDelete={item => void remove(item)} now={now + clockOffset.current} onCompose={() => setCompose(true)} />
          {!online && <div className="offline-banner" role="status">接続を回復しています。復帰すると最新の状態に更新されます。</div>}
          {latest && Date.parse(latest.createdAt) > seen && <button className="new-item" onClick={jump}>新しいカードへ ↗</button>}
          <div className="zoom-tools" aria-label="表示倍率"><button aria-label="縮小" onClick={() => zoom(1 / 1.2)}>−</button><button className="zoom-value" aria-label="表示をリセット" onClick={() => onViewport(initialViewport)}>{Math.round(viewport.zoom * 100)}%</button><button aria-label="拡大" onClick={() => zoom(1.2)}>＋</button></div>
          <div className="canvas-caption">背景をドラッグで移動 · ホイールでズーム</div>
        </main>
        <footer className="app-footer"><span><span className="status-dot" /> RAMだけに保存</span><span>{((board?.memoryBytes ?? 0) / (1 << 20)).toFixed(1)} / {(board?.limits.maxMemoryBytes ?? 200 << 20) / (1 << 20)} MB <span className="footer-separator">·</span> 期限になると自動削除</span></footer>
        <input className="visually-hidden" aria-label="画像ファイル" type="file" accept="image/png,image/jpeg,image/gif" ref={fileInput} onChange={e => { const file = e.target.files?.[0]; if (file) void addImage(file); e.target.value = '' }} />
      </>}
    {compose && <Modal title="テキストを置く" close={() => setCompose(false)}><form onSubmit={e => { e.preventDefault(); void addText(text) }}><label htmlFor="text">共有するテキスト</label><textarea id="text" autoFocus value={text} onChange={e => setText(e.target.value)} placeholder="URL、コマンド、メモなどを貼り付け" rows={7} /><p className="form-note">プレーンテキストとして共有されます。作成時から有効期限が始まります。</p><div className="modal-actions"><button type="button" onClick={() => setCompose(false)}>キャンセル</button><button className="primary" disabled={busy || !online || !text.trim()}>共有する →</button></div></form></Modal>}
    {clear && <Modal title="すべてのカードを削除しますか？" close={() => setClear(false)}><p>接続中のすべての端末からテキストと画像が消えます。この操作は取り消せません。</p><div className="modal-actions"><button onClick={() => setClear(false)}>キャンセル</button><button className="danger" disabled={busy || !online} onClick={() => void clearBoard()}>すべて削除</button></div></Modal>}
    {help && <Modal title="キャンバスの使い方" close={() => setHelp(false)}><dl className="help-list"><dt>貼り付け</dt><dd>Ctrl＋V。スマートフォンではテキスト入力や画像選択を使えます。</dd><dt>カード移動</dt><dd>カード上部をドラッグ。本文は選択してコピーできます。</dd><dt>サイズ変更</dt><dd>選択したカードの右下をドラッグ。キーボードではサイズ変更ボタンにフォーカスして矢印キー。</dd><dt>表示移動</dt><dd>背景をドラッグ。ホイールでズーム。スマートフォンでは2本指で移動・拡大縮小。</dd><dt>取り出す</dt><dd>コピーを押すか、選択してCtrl＋C。画像は「開く」「保存」も使えます。長押しでコピーメニュー。</dd><dt>期限・プライバシー</dt><dd>初期設定では1時間で自動削除。サーバー終了でもすべて消えます。</dd></dl></Modal>}
    {notice && <div className="toast" role="status" aria-live="polite">{notice}<button aria-label="通知を閉じる" onClick={() => setNotice('')}>×</button></div>}
    {busy && <div className="busy-indicator" role="status">送信中…</div>}
  </div>;
}

function Modal({ title, close, children }: { title: string; close: () => void; children: React.ReactNode }) {
  const dialog = useRef<HTMLDialogElement>(null);
  useEffect(() => { const el = dialog.current!; el.showModal(); return () => el.close() }, []);
  return <dialog ref={dialog} className="modal" aria-labelledby="modal-title" onCancel={close} onClick={e => { if (e.target === e.currentTarget) { const r = e.currentTarget.getBoundingClientRect(); if (e.clientX < r.left || e.clientX > r.right || e.clientY < r.top || e.clientY > r.bottom) close() } }}><div className="modal-heading"><h2 id="modal-title">{title}</h2><button aria-label="閉じる" onClick={close}>×</button></div>{children}</dialog>;
}
