import { createContext, useCallback, useContext, useEffect, useMemo, useRef, useState, type ReactNode } from 'react';
import { api } from '../api/client';
import { useAuth } from '../auth/AuthProvider';
import {
  indexedDBStore,
  isOutstanding,
  makeItem,
  markForRetry,
  processQueue,
  type NewItem,
  type QueueItem,
  type QueueStore,
} from './queue';

const RETRY_EVERY_MS = 30_000;

interface QueueCtx {
  items: QueueItem[];
  outstanding: number;
  online: boolean;
  /** Saves locally first, then tries to send. Resolves to null when sent, else the item still queued. */
  submit: (n: NewItem) => Promise<QueueItem | null>;
  retry: (key: string) => Promise<void>;
  discard: (key: string) => Promise<void>;
  sendNow: () => Promise<void>;
  clearAll: () => Promise<void>;
}

const Ctx = createContext<QueueCtx | null>(null);

function memoryStore(): QueueStore {
  const m = new Map<string, QueueItem>();
  return {
    all: async () => [...m.values()],
    put: async (i) => void m.set(i.key, i),
    delete: async (k) => void m.delete(k),
    clear: async () => m.clear(),
  };
}

function openStore(): QueueStore {
  try {
    if (typeof indexedDB !== 'undefined') return indexedDBStore();
  } catch {
    // private mode without IndexedDB: fall back to memory (survives only this tab)
  }
  return memoryStore();
}

const send = (item: QueueItem) =>
  api.request(item.method, item.path, { body: item.body, idempotencyKey: item.key });

export function QueueProvider({ children }: { children: ReactNode }) {
  const store = useRef<QueueStore>(openStore());
  const [items, setItems] = useState<QueueItem[]>([]);
  const [online, setOnline] = useState(() => (typeof navigator === 'undefined' ? true : navigator.onLine));
  const { state } = useAuth();
  const authReady = state.kind === 'ready';

  const refresh = useCallback(async () => {
    try {
      const all = await store.current.all();
      setItems(all.sort((a, b) => a.createdAt - b.createdAt));
    } catch {
      setItems([]);
    }
  }, []);

  const flush = useCallback(
    async (only?: string[]) => {
      try {
        await processQueue(store.current, send, { authReady, only });
      } finally {
        await refresh();
      }
    },
    [authReady, refresh],
  );

  useEffect(() => {
    void refresh();
  }, [refresh]);

  // Retry on reconnect, on return to the app, every 30 s, and after unlock/login.
  useEffect(() => {
    if (!authReady) return;
    void flush();
    const onOnline = () => {
      setOnline(true);
      void flush();
    };
    const onOffline = () => setOnline(false);
    const onVisible = () => {
      if (document.visibilityState === 'visible') void flush();
    };
    window.addEventListener('online', onOnline);
    window.addEventListener('offline', onOffline);
    document.addEventListener('visibilitychange', onVisible);
    const timer = window.setInterval(() => void flush(), RETRY_EVERY_MS);
    return () => {
      window.removeEventListener('online', onOnline);
      window.removeEventListener('offline', onOffline);
      document.removeEventListener('visibilitychange', onVisible);
      window.clearInterval(timer);
    };
  }, [authReady, flush]);

  const submit = useCallback(
    async (n: NewItem) => {
      await store.current.put(makeItem(n, Date.now()));
      await refresh();
      await processQueue(store.current, send, { authReady: true, only: [n.key] });
      await refresh();
      return (await store.current.all()).find((i) => i.key === n.key) ?? null;
    },
    [refresh],
  );

  const value = useMemo<QueueCtx>(
    () => ({
      items,
      outstanding: items.filter(isOutstanding).length,
      online,
      submit,
      retry: async (key) => {
        await markForRetry(store.current, key);
        await flush([key]);
      },
      discard: async (key) => {
        await store.current.delete(key);
        await refresh();
      },
      sendNow: () => flush(),
      clearAll: async () => {
        await store.current.clear();
        await refresh();
      },
    }),
    [items, online, submit, flush, refresh],
  );

  return <Ctx.Provider value={value}>{children}</Ctx.Provider>;
}

export function useQueue(): QueueCtx {
  const v = useContext(Ctx);
  if (!v) throw new Error('useQueue outside QueueProvider');
  return v;
}

/** The queued (not yet sent) items for one hearing. */
export function useQueuedFor(hearingId: string): QueueItem[] {
  const { items } = useQueue();
  return items.filter((i) => i.hearingId === hearingId);
}
