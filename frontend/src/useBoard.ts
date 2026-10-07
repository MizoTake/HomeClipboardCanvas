import { useEffect, useRef, useState } from 'react';
import { api, APIError, type Board, type BoardEvent, jsonRequest } from './model';

export function useBoard(notify: (message: string) => void) {
  const [board, setBoard] = useState<Board | null>(null);
  const [authenticated, setAuthenticated] = useState(false);
  const [checking, setChecking] = useState(true);
  const [online, setOnline] = useState(false);
  const [initial, setInitial] = useState<Board | null>(null);
  const clockOffset = useRef(0);
  const login = async (token: string) => {
    await api('/api/session', jsonRequest('POST', { token }));
    const snapshot = await api<Board>('/api/board');
    setBoard(snapshot); setInitial(snapshot); setAuthenticated(true);
    clockOffset.current = Date.parse(snapshot.serverTime) - Date.now();
  };

  useEffect(() => {
    let active = true;
    const url = new URL(location.href);
    const token = url.searchParams.get('token');
    if (token !== null) { url.searchParams.delete('token'); history.replaceState(null, '', url.pathname + url.search + url.hash) }
    async function bootstrap() {
      try {
        if (token) await api('/api/session', jsonRequest('POST', { token }));
        const snapshot = await api<Board>('/api/board');
        if (active) { setBoard(snapshot); setInitial(snapshot); setAuthenticated(true); clockOffset.current = Date.parse(snapshot.serverTime) - Date.now() }
      } catch (error) {
        if (active && (!(error instanceof APIError) || error.status !== 401 || token)) notify(error instanceof Error ? error.message : '接続できませんでした。');
      } finally { if (active) setChecking(false) }
    }
    void bootstrap();
    return () => { active = false };
  }, []);

  useEffect(() => {
    if (!authenticated) return;
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
          setBoard(message.snapshot); setOnline(true); attempt = 0;
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
      current.onclose = async () => {
        if (stopped || socket !== current) return;
        setOnline(false);
        if (!navigator.onLine) return;
        try { await api('/api/board') } catch (error) {
          if (stopped || socket !== current) return;
          if (error instanceof APIError && error.status === 401) { setAuthenticated(false); setBoard(null); notify(error.message); return }
        }
        if (!stopped && socket === current) timer = setTimeout(connect, Math.min(1000 * 2 ** attempt++, 15000));
      };
    };
    const offline = () => { setOnline(false); socket?.close() };
    const online = () => { if (socket?.readyState === WebSocket.OPEN || socket?.readyState === WebSocket.CONNECTING) return; connect() };
    addEventListener('offline', offline); addEventListener('online', online);
    connect();
    return () => { stopped = true; clearTimeout(timer); socket?.close(); removeEventListener('offline', offline); removeEventListener('online', online) };
  }, [authenticated]);

  const resetAuth = () => { setAuthenticated(false); setBoard(null) };
  return { board, authenticated, checking, online, initial, login, clockOffset, resetAuth };
}
