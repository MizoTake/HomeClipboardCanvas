import { useEffect, useRef, useState } from 'react';
import type { Board, BoardEvent } from './model';

export function useBoard() {
  const [board, setBoard] = useState<Board | null>(null);
  const [checking, setChecking] = useState(true);
  const [online, setOnline] = useState(false);
  const [initial, setInitial] = useState<Board | null>(null);
  const clockOffset = useRef(0);
  const receivedInitial = useRef(false);

  useEffect(() => {
    let stopped = false;
    let socket: WebSocket | undefined;
    let timer: ReturnType<typeof setTimeout>;
    let attempt = 0;
    const connect = () => {
      if (stopped || !navigator.onLine) return;
      clearTimeout(timer);
      const current = new WebSocket(`${location.protocol === 'https:' ? 'wss:' : 'ws:'}//${location.host}/api/ws`);
      socket = current;
      current.onmessage = event => {
        if (stopped || socket !== current) return;
        const message = JSON.parse(event.data) as BoardEvent;
        if (message.type === 'board.snapshot' && message.snapshot) {
          clockOffset.current = Date.parse(message.snapshot.serverTime) - Date.now();
          setBoard(message.snapshot); setOnline(true); setChecking(false); attempt = 0;
          // 再接続時に利用中の表示位置を初期位置へ戻さない。
          if (!receivedInitial.current) { receivedInitial.current = true; setInitial(message.snapshot) }
          return;
        }
        setBoard(previous => {
          if (!previous) return previous;
          const next = { ...previous, memoryBytes: message.memoryBytes };
          if (message.type === 'devices.updated') next.devices = message.devices ?? 0;
          if ((message.type === 'item.created' || message.type === 'item.updated') && message.item) {
            const exists = previous.items.find(item => item.id === message.item!.id);
            if (!exists || exists.revision <= message.item.revision) next.items = [...previous.items.filter(item => item.id !== message.item!.id), message.item];
            if (message.type === 'item.created') { next.lastCreatedItemId = message.item.id; next.lastCreatedAt = message.item.createdAt }
          }
          if (message.type === 'item.deleted' || message.type === 'item.expired') next.items = previous.items.filter(item => item.id !== message.id);
          if (message.type === 'board.cleared') { next.items = []; next.lastCreatedItemId = ''; next.lastCreatedAt = '' }
          return next;
        });
      };
      current.onclose = () => {
        if (stopped || socket !== current) return;
        setOnline(false); setChecking(false);
        if (!navigator.onLine) return;
        timer = setTimeout(connect, Math.min(1000 * 2 ** attempt++, 15000));
      };
    };
    const offline = () => { setOnline(false); socket?.close() };
    const online = () => { if (socket?.readyState === WebSocket.OPEN || socket?.readyState === WebSocket.CONNECTING) return; connect() };
    addEventListener('offline', offline); addEventListener('online', online);
    connect();
    return () => { stopped = true; clearTimeout(timer); socket?.close(); removeEventListener('offline', offline); removeEventListener('online', online) };
  }, []);

  return { board, ready: board !== null, checking, online, initial, clockOffset };
}
