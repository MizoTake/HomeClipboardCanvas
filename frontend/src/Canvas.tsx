import { useEffect, useRef, useState, type CSSProperties, type PointerEvent } from 'react';
import { clamp, imageURL, zoomAt, type Geometry, type Item, type Viewport } from './model';

interface Props {
  items: Item[];
  viewport: Viewport;
  onViewport: (v: Viewport) => void;
  selected: string | null;
  onSelect: (id: string | null) => void;
  onUpdate: (item: Item, geometry: Geometry) => void;
  onCopy: (item: Item) => void;
  onDelete: (item: Item) => void;
  now: number;
  onCompose: () => void;
}
type Gesture = { kind: 'pan' | 'move' | 'resize'; startX: number; startY: number; viewport: Viewport; item?: Item; moved: boolean };

export function Canvas(props: Props) {
  const { viewport, selected, onSelect, onViewport } = props;
  const area = useRef<HTMLDivElement>(null);
  const gesture = useRef<Gesture | null>(null);
  const points = useRef(new Map<number, { x: number; y: number }>());
  const pinch = useRef<{ distance: number; center: { x: number; y: number }; viewport: Viewport } | null>(null);
  const longPress = useRef<ReturnType<typeof setTimeout> | undefined>(undefined);
  const [draft, setDraft] = useState<{ id: string; geometry: Geometry } | null>(null);
  const draftRef = useRef<{ id: string; geometry: Geometry } | null>(null);
  const [menu, setMenu] = useState<string | null>(null);
  const [panning, setPanning] = useState(false);
  const viewRef = useRef(viewport);
  viewRef.current = viewport;

  useEffect(() => {
    const el = area.current!;
    const wheel = (e: WheelEvent) => {
      e.preventDefault();
      const rect = el.getBoundingClientRect();
      onViewport(zoomAt(viewRef.current, viewRef.current.zoom * Math.exp(-e.deltaY * .0015), { x: e.clientX - rect.left, y: e.clientY - rect.top }));
    };
    el.addEventListener('wheel', wheel, { passive: false });
    return () => el.removeEventListener('wheel', wheel);
  }, [onViewport]);
  useEffect(() => () => clearTimeout(longPress.current), []);

  const begin = (e: PointerEvent, kind: Gesture['kind'], item?: Item) => {
    if (e.button !== 0 || (kind !== 'resize' && (e.target as HTMLElement).closest('button,a,textarea,input'))) return;
    if (kind !== 'pan') e.stopPropagation();
    const rect = area.current!.getBoundingClientRect();
    points.current.set(e.pointerId, { x: e.clientX - rect.left, y: e.clientY - rect.top });
    area.current!.setPointerCapture(e.pointerId);
    if (points.current.size === 2) {
      clearTimeout(longPress.current);
      const [a, b] = [...points.current.values()];
      pinch.current = { distance: Math.hypot(a.x - b.x, a.y - b.y), center: { x: (a.x + b.x) / 2, y: (a.y + b.y) / 2 }, viewport };
      gesture.current = null; draftRef.current = null; setDraft(null); setPanning(true); return;
    }
    gesture.current = { kind, startX: e.clientX, startY: e.clientY, viewport, item, moved: false };
    setMenu(null);
    if (item) {
      onSelect(item.id);
      if (e.pointerType === 'touch' && kind === 'move') longPress.current = setTimeout(() => { setMenu(item.id); gesture.current = null }, 550);
    } else { onSelect(null); setPanning(true) }
  };
  const move = (e: PointerEvent) => {
    if (!points.current.has(e.pointerId)) return;
    const rect = area.current!.getBoundingClientRect();
    points.current.set(e.pointerId, { x: e.clientX - rect.left, y: e.clientY - rect.top });
    if (pinch.current && points.current.size >= 2) {
      const [a, b] = [...points.current.values()];
      const center = { x: (a.x + b.x) / 2, y: (a.y + b.y) / 2 };
      const base = pinch.current;
      const next = zoomAt(base.viewport, base.viewport.zoom * Math.hypot(a.x - b.x, a.y - b.y) / Math.max(base.distance, 1), base.center);
      onViewport({ ...next, x: next.x + center.x - base.center.x, y: next.y + center.y - base.center.y }); return;
    }
    const g = gesture.current;
    if (!g) return;
    const dx = e.clientX - g.startX, dy = e.clientY - g.startY;
    if (Math.hypot(dx, dy) > 5) { g.moved = true; clearTimeout(longPress.current) }
    if (!g.moved) return;
    if (g.kind === 'pan') onViewport({ ...g.viewport, x: g.viewport.x + dx, y: g.viewport.y + dy });
    else if (g.item) {
      const item = g.item;
      const geometry = g.kind === 'move'
        ? { x: item.x + dx / g.viewport.zoom, y: item.y + dy / g.viewport.zoom, width: item.width, height: item.height }
        : { x: item.x, y: item.y, width: clamp(item.width + dx / g.viewport.zoom, 200, 4096), height: clamp(item.height + dy / g.viewport.zoom, 140, 4096) };
      draftRef.current = { id: item.id, geometry };
      setDraft(draftRef.current);
    }
  };
  const finish = (e: PointerEvent, cancelled = false) => {
    clearTimeout(longPress.current);
    points.current.delete(e.pointerId);
    if (area.current?.hasPointerCapture(e.pointerId)) area.current.releasePointerCapture(e.pointerId);
    if (pinch.current) {
      pinch.current = null; gesture.current = null; points.current.clear(); setPanning(false); return;
    }
    const g = gesture.current;
    // 同一描画フレーム内のtouchmove・touchendで、未描画の最終座標を捨てない。
    if (!cancelled && g?.item && g.moved && draftRef.current) props.onUpdate(g.item, draftRef.current.geometry);
    gesture.current = null; draftRef.current = null; setDraft(null); setPanning(false);
  };
  const style = { '--grid': `${24 * viewport.zoom}px`, backgroundPosition: `${viewport.x}px ${viewport.y}px` } as CSSProperties;

  return <div ref={area} className={`canvas ${panning ? 'is-panning' : ''}`} data-testid="canvas" style={style}
    onPointerDown={e => begin(e, 'pan')} onPointerMove={move} onPointerUp={e => finish(e)} onPointerCancel={e => finish(e, true)}
    onContextMenu={e => { if (!(e.target as HTMLElement).closest('.card')) e.preventDefault() }}>
    <div className="canvas-plane" data-testid="canvas-plane" style={{ transform: `translate(${viewport.x}px, ${viewport.y}px) scale(${viewport.zoom})` }}>
      {props.items.map(item => {
        const g = draft?.id === item.id ? draft.geometry : item;
        const minutes = Math.max(0, Math.min(Math.ceil((Date.parse(item.expiresAt) - Date.parse(item.createdAt)) / 60000), Math.ceil((Date.parse(item.expiresAt) - props.now) / 60000)));
        return <article key={item.id} className={`card ${item.type} ${selected === item.id ? 'selected' : ''}`} data-item-id={item.id}
          tabIndex={0} onFocus={() => onSelect(item.id)}
          onKeyDown={e => { if (e.target !== e.currentTarget || !['ArrowLeft', 'ArrowRight', 'ArrowUp', 'ArrowDown'].includes(e.key)) return; e.preventDefault(); props.onUpdate(item, { ...item, x: item.x + (e.key === 'ArrowRight' ? 20 : e.key === 'ArrowLeft' ? -20 : 0), y: item.y + (e.key === 'ArrowDown' ? 20 : e.key === 'ArrowUp' ? -20 : 0) }) }}
          aria-label={item.type === 'text' ? 'テキストカード' : '画像カード'} style={{ left: g.x, top: g.y, width: g.width, height: g.height }}
          onPointerDown={e => begin(e, 'move', item)} onContextMenu={e => { e.preventDefault(); setMenu(item.id); onSelect(item.id) }}>
          <div className="card-grip" aria-hidden="true"><span>{item.type === 'text' ? 'TEXT' : 'IMAGE'}</span><span>⠿</span></div>
          {item.type === 'text' ? <pre className="card-content" tabIndex={0} onPointerDown={e => { if (e.pointerType === 'mouse') { e.stopPropagation(); onSelect(item.id) } }}>{item.text}</pre>
            : <div className="image-content"><img src={imageURL(item)} alt="共有された画像" draggable={false} /></div>}
          <footer className="card-footer">
            <span className={minutes <= 5 ? 'expiring' : ''} title="作成時からの有効期限。移動しても延長されません。">◷ {minutes > 0 ? `残り${minutes}分` : '期限切れ'}</span>
            <button onClick={() => props.onCopy(item)} aria-label={item.type === 'text' ? 'テキストをコピー' : '画像をコピー'}>コピー</button>
            <button className="delete-item" onClick={() => props.onDelete(item)} aria-label="カードを削除">×</button>
          </footer>
          {item.type === 'image' && <div className="image-actions"><a href={imageURL(item)} target="_blank" rel="noopener noreferrer">開く ↗</a><a href={imageURL(item)} download={`clipboard-${item.id}.${item.mimeType === 'image/jpeg' ? 'jpg' : item.mimeType === 'image/gif' ? 'gif' : 'png'}`}>保存 ↓</a></div>}
          {selected === item.id && <button className="resize-handle" aria-label="カードのサイズ変更" onPointerDown={e => begin(e, 'resize', item)}
            onKeyDown={e => { if (!['ArrowLeft', 'ArrowRight', 'ArrowUp', 'ArrowDown'].includes(e.key)) return; e.preventDefault(); props.onUpdate(item, { ...item, width: clamp(item.width + (e.key === 'ArrowRight' ? 20 : e.key === 'ArrowLeft' ? -20 : 0), 200, 4096), height: clamp(item.height + (e.key === 'ArrowDown' ? 20 : e.key === 'ArrowUp' ? -20 : 0), 140, 4096) }) }} />}
          {menu === item.id && <div className="copy-menu" role="menu"><button role="menuitem" onClick={() => { props.onCopy(item); setMenu(null) }}>コピー</button><button role="menuitem" onClick={() => setMenu(null)}>閉じる</button></div>}
        </article>;
      })}
    </div>
    {props.items.length === 0 && <div className="empty-board" onPointerDown={e => e.stopPropagation()}>
      <div className="empty-mark" aria-hidden="true"><span>↗</span><span>↙</span></div>
      <p className="eyebrow">A LITTLE SPACE TO SHARE</p>
      <h2>コピーしたものを、<br />ここに置いておこう。</h2>
      <p>テキストも、スクリーンショットも。<br />同じLANの端末へ、そのまま渡せます。</p>
      <button className="primary" onClick={props.onCompose}>＋ テキストを置く</button>
      <span className="paste-hint">または <kbd>Ctrl</kbd> + <kbd>V</kbd> で貼り付け</span>
    </div>}
  </div>;
}
