// Offline-tolerant queue for court-day writes (doc 12 §3.4, §5.2; ADR 0008).
//
// An outcome or note is written to IndexedDB first, with its Idempotency-Key
// and the time the user recorded it, and only then sent. The API stores the
// first response for a key, so a resend after a lost reply is replayed, not
// duplicated. Rules:
//   2xx (incl. replayed)          → remove
//   network / 5xx / 429 / in-progress → keep, retry later automatically
//   401                           → keep, retry after unlock / login
//   409 / 412 / 422 / other 4xx   → keep, show to the user, never auto-retry
//   older than 24 h               → expire (body wiped, tombstone kept for the user)

import { ApiError, isRetryable } from '../api/client';

export const QUEUE_TTL_MS = 24 * 60 * 60 * 1000;
export const EXPIRY_WARNING_MS = 3 * 60 * 60 * 1000;

export type QueueKind = 'outcome' | 'note';

export type QueueStatus =
  | 'pending' // not yet tried, or due for a retry
  | 'waiting_network' // last try failed for connectivity; retried automatically
  | 'waiting_auth' // last try got 401; retried after unlock / login
  | 'needs_attention' // rejected (409/412/422…); the user decides
  | 'expired'; // not sent within 24 h; body removed

export interface QueueError {
  status: number;
  code: string;
  requestId: string;
  fields?: { field: string; code: string }[];
}

export interface QueueItem {
  /** The Idempotency-Key; also the primary key. */
  key: string;
  kind: QueueKind;
  hearingId: string;
  method: 'PUT' | 'POST';
  path: string;
  /** Minimal request body; null once expired. */
  body: unknown;
  createdAt: number;
  expiresAt: number;
  status: QueueStatus;
  attempts: number;
  lastAttemptAt?: number;
  error?: QueueError;
  /** What the UI shows about the item (no client names). */
  label: { caseNumber?: string; courtDate?: string; nextDate?: string };
}

export interface QueueStore {
  all(): Promise<QueueItem[]>;
  put(item: QueueItem): Promise<void>;
  delete(key: string): Promise<void>;
  clear(): Promise<void>;
}

export type Sender = (item: QueueItem) => Promise<unknown>;

export interface NewItem {
  key: string;
  kind: QueueKind;
  hearingId: string;
  method: 'PUT' | 'POST';
  path: string;
  body: unknown;
  label?: QueueItem['label'];
}

export function makeItem(n: NewItem, now: number): QueueItem {
  return {
    ...n,
    label: n.label ?? {},
    createdAt: now,
    expiresAt: now + QUEUE_TTL_MS,
    status: 'pending',
    attempts: 0,
  };
}

export function isExpiringSoon(item: QueueItem, now: number): boolean {
  return item.status !== 'expired' && item.expiresAt - now <= EXPIRY_WARNING_MS;
}

export function isOutstanding(item: QueueItem): boolean {
  return item.status !== 'expired';
}

export interface ProcessOptions {
  now?: number;
  /** False while the session is locked or logged out: 401-held items wait. */
  authReady?: boolean;
  /** Only these keys (e.g. "send now" for one item). */
  only?: string[];
  /** When the browser reports offline, nothing is sent (expiry still runs). */
  online?: boolean;
}

export interface ProcessReport {
  sent: string[];
  failed: string[];
  expired: string[];
}

/** Turns a stored error back into an ApiError so the UI can translate it. */
export function queueErrorAsApiError(e: QueueError | undefined): ApiError {
  return new ApiError(e?.status ?? 0, e?.code ?? 'unknown', e?.requestId ?? '', {}, e?.fields ?? []);
}

function toQueueError(err: ApiError): QueueError {
  return {
    status: err.status,
    code: err.code,
    requestId: err.requestId,
    fields: err.fields.map((f) => ({ field: f.field, code: f.code })),
  };
}

/** Marks items older than 24 h as expired and wipes their body. */
export async function expireOld(store: QueueStore, now: number): Promise<string[]> {
  const out: string[] = [];
  for (const item of await store.all()) {
    if (item.status !== 'expired' && now >= item.expiresAt) {
      await store.put({ ...item, status: 'expired', body: null });
      out.push(item.key);
    }
  }
  return out;
}

let chain: Promise<unknown> = Promise.resolve();

/**
 * Sends every eligible item once, oldest first. Runs are serialised, so an
 * item is never sent twice at the same time (two tabs share the key, and the
 * API answers a concurrent duplicate with idempotency.in_progress → retry).
 */
export function processQueue(store: QueueStore, send: Sender, opts: ProcessOptions = {}): Promise<ProcessReport> {
  const next = chain.then(() => run(store, send, opts));
  chain = next.catch(() => undefined);
  return next;
}

async function run(store: QueueStore, send: Sender, opts: ProcessOptions): Promise<ProcessReport> {
  const now = opts.now ?? Date.now();
  if (opts.online === false) return { sent: [], failed: [], expired: await expireOld(store, now) };
  const report: ProcessReport = { sent: [], failed: [], expired: await expireOld(store, now) };
  const items = (await store.all()).sort((a, b) => a.createdAt - b.createdAt);
  for (const item of items) {
    if (opts.only && !opts.only.includes(item.key)) continue;
    if (item.status === 'expired' || item.status === 'needs_attention') continue;
    if (item.status === 'waiting_auth' && opts.authReady === false) continue;
    const attempt = { ...item, attempts: item.attempts + 1, lastAttemptAt: now };
    try {
      await send(attempt);
      await store.delete(item.key);
      report.sent.push(item.key);
    } catch (err) {
      report.failed.push(item.key);
      if (err instanceof ApiError && err.status === 401) {
        await store.put({ ...attempt, status: 'waiting_auth', error: toQueueError(err) });
      } else if (isRetryable(err)) {
        const error = err instanceof ApiError ? toQueueError(err) : undefined;
        await store.put({ ...attempt, status: 'waiting_network', error });
      } else if (err instanceof ApiError) {
        await store.put({ ...attempt, status: 'needs_attention', error: toQueueError(err) });
      } else {
        // Unknown failure (bug, quota): keep the data and let the user decide.
        await store.put({ ...attempt, status: 'needs_attention', error: { status: 0, code: 'unknown', requestId: '' } });
      }
    }
  }
  return report;
}

/** The user chose to try a rejected item again (after fixing the cause). */
export async function markForRetry(store: QueueStore, key: string): Promise<void> {
  const item = (await store.all()).find((i) => i.key === key);
  if (item && item.status !== 'expired') await store.put({ ...item, status: 'pending' });
}

// ---------------------------------------------------------------------------
// IndexedDB store
// ---------------------------------------------------------------------------

const DB_NAME = 'nithidesk';
const DB_VERSION = 1;
const STORE = 'outbox';

function promisify<T>(req: IDBRequest<T>): Promise<T> {
  return new Promise((resolve, reject) => {
    req.onsuccess = () => resolve(req.result);
    req.onerror = () => reject(req.error);
  });
}

function txDone(tx: IDBTransaction): Promise<void> {
  return new Promise((resolve, reject) => {
    tx.oncomplete = () => resolve();
    tx.onerror = () => reject(tx.error);
    tx.onabort = () => reject(tx.error);
  });
}

export function indexedDBStore(factory: IDBFactory = globalThis.indexedDB, name = DB_NAME): QueueStore {
  let dbp: Promise<IDBDatabase> | null = null;
  const open = () => {
    if (!dbp) {
      dbp = new Promise((resolve, reject) => {
        const req = factory.open(name, DB_VERSION);
        req.onupgradeneeded = () => {
          const db = req.result;
          if (!db.objectStoreNames.contains(STORE)) db.createObjectStore(STORE, { keyPath: 'key' });
        };
        req.onsuccess = () => resolve(req.result);
        req.onerror = () => reject(req.error);
      });
    }
    return dbp;
  };
  return {
    async all() {
      const db = await open();
      const tx = db.transaction(STORE, 'readonly');
      return (await promisify(tx.objectStore(STORE).getAll())) as QueueItem[];
    },
    async put(item) {
      const db = await open();
      const tx = db.transaction(STORE, 'readwrite');
      tx.objectStore(STORE).put(item);
      await txDone(tx);
    },
    async delete(key) {
      const db = await open();
      const tx = db.transaction(STORE, 'readwrite');
      tx.objectStore(STORE).delete(key);
      await txDone(tx);
    },
    async clear() {
      const db = await open();
      const tx = db.transaction(STORE, 'readwrite');
      tx.objectStore(STORE).clear();
      await txDone(tx);
    },
  };
}
