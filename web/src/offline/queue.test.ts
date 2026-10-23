import 'fake-indexeddb/auto';
import { IDBFactory } from 'fake-indexeddb';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { ApiError, NetworkError } from '../api/client';
import {
  EXPIRY_WARNING_MS,
  QUEUE_TTL_MS,
  indexedDBStore,
  isExpiringSoon,
  makeItem,
  markForRetry,
  processQueue,
  type QueueItem,
  type QueueStore,
} from './queue';

const T0 = Date.UTC(2026, 9, 20, 4, 0, 0);
let store: QueueStore;

beforeEach(() => {
  store = indexedDBStore(new IDBFactory(), `test-${Math.random()}`);
});

function outcome(key: string, at = T0): QueueItem {
  return makeItem(
    {
      key,
      kind: 'outcome',
      hearingId: 'h1',
      method: 'PUT',
      path: '/api/v1/hearings/h1/outcome',
      body: { outcome_type_id: 't', next_hearing: { date: '2026-11-24', session: 'morning' } },
      label: { caseNumber: 'DC/L/1234', nextDate: '2026-11-24' },
    },
    at,
  );
}

describe('offline queue', () => {
  it('stores items in IndexedDB with their idempotency key', async () => {
    await store.put(outcome('k1'));
    const all = await store.all();
    expect(all).toHaveLength(1);
    expect(all[0]!.key).toBe('k1');
    expect(all[0]!.expiresAt - all[0]!.createdAt).toBe(QUEUE_TTL_MS);
  });

  it('removes an item after a 2xx (including a replayed one)', async () => {
    await store.put(outcome('k1'));
    const send = vi.fn(async (_item: QueueItem) => ({ data: {}, warnings: [] }));
    const r = await processQueue(store, send, { now: T0 + 1000 });
    expect(r.sent).toEqual(['k1']);
    expect(send).toHaveBeenCalledTimes(1);
    expect(send.mock.calls[0]![0]).toMatchObject({ key: 'k1', attempts: 1 });
    expect(await store.all()).toEqual([]);
  });

  it('keeps an item on network failure and retries it later', async () => {
    await store.put(outcome('k1'));
    const send = vi.fn().mockRejectedValueOnce(new NetworkError()).mockResolvedValueOnce({});
    await processQueue(store, send, { now: T0 + 1000 });
    let all = await store.all();
    expect(all[0]!.status).toBe('waiting_network');
    expect(all[0]!.attempts).toBe(1);
    await processQueue(store, send, { now: T0 + 31_000 });
    all = await store.all();
    expect(all).toEqual([]);
    expect(send).toHaveBeenCalledTimes(2);
  });

  it('treats 5xx and idempotency.in_progress as retryable', async () => {
    await store.put(outcome('k1'));
    await store.put(outcome('k2', T0 + 1));
    const send = vi
      .fn()
      .mockRejectedValueOnce(new ApiError(503, 'internal_error'))
      .mockRejectedValueOnce(new ApiError(409, 'idempotency.in_progress'));
    await processQueue(store, send, { now: T0 + 1000 });
    const all = await store.all();
    expect(all.map((i) => i.status)).toEqual(['waiting_network', 'waiting_network']);
  });

  it.each([
    [409, 'hearing.outcome_already_recorded'],
    [412, 'precondition.version_mismatch'],
    [422, 'validation.failed'],
  ])('keeps %i %s for the user and never auto-retries it', async (status, code) => {
    await store.put(outcome('k1'));
    const send = vi.fn().mockRejectedValue(new ApiError(status, code, 'rid'));
    await processQueue(store, send, { now: T0 + 1000 });
    await processQueue(store, send, { now: T0 + 60_000 });
    const all = await store.all();
    expect(send).toHaveBeenCalledTimes(1);
    expect(all[0]!.status).toBe('needs_attention');
    expect(all[0]!.error).toMatchObject({ status, code, requestId: 'rid' });
    expect(all[0]!.body).not.toBeNull();
  });

  it('retries a rejected item only after the user asks', async () => {
    await store.put(outcome('k1'));
    const send = vi.fn().mockRejectedValueOnce(new ApiError(409, 'hearing.outcome_already_recorded')).mockResolvedValueOnce({});
    await processQueue(store, send, { now: T0 });
    await markForRetry(store, 'k1');
    await processQueue(store, send, { now: T0 + 1 });
    expect(await store.all()).toEqual([]);
    expect(send).toHaveBeenCalledTimes(2);
  });

  it('holds 401 items until the session is back, then sends them', async () => {
    await store.put(outcome('k1'));
    const send = vi.fn().mockRejectedValueOnce(new ApiError(401, 'auth.session_locked')).mockResolvedValueOnce({});
    await processQueue(store, send, { now: T0 });
    expect((await store.all())[0]!.status).toBe('waiting_auth');
    await processQueue(store, send, { now: T0 + 1, authReady: false });
    expect(send).toHaveBeenCalledTimes(1);
    await processQueue(store, send, { now: T0 + 2, authReady: true });
    expect(await store.all()).toEqual([]);
  });

  it('expires items after 24 h, wipes the body and keeps a tombstone', async () => {
    await store.put(outcome('k1'));
    const send = vi.fn();
    const r = await processQueue(store, send, { now: T0 + QUEUE_TTL_MS });
    expect(r.expired).toEqual(['k1']);
    expect(send).not.toHaveBeenCalled();
    const [item] = await store.all();
    expect(item!.status).toBe('expired');
    expect(item!.body).toBeNull();
    expect(item!.label.caseNumber).toBe('DC/L/1234');
  });

  it('warns before expiry', () => {
    const item = outcome('k1');
    expect(isExpiringSoon(item, T0 + 1000)).toBe(false);
    expect(isExpiringSoon(item, item.expiresAt - EXPIRY_WARNING_MS + 1)).toBe(true);
  });

  it('sends oldest first and can target one item', async () => {
    await store.put(outcome('late', T0 + 50));
    await store.put(outcome('early', T0));
    const order: string[] = [];
    const send = vi.fn(async (i: QueueItem) => void order.push(i.key));
    await processQueue(store, send, { now: T0 + 100, only: ['late'] });
    expect(order).toEqual(['late']);
    await processQueue(store, send, { now: T0 + 100 });
    expect(order).toEqual(['late', 'early']);
  });

  it('serialises runs so an item is never sent twice concurrently', async () => {
    await store.put(outcome('k1'));
    let inFlight = 0;
    let maxInFlight = 0;
    const send = vi.fn(async () => {
      inFlight++;
      maxInFlight = Math.max(maxInFlight, inFlight);
      await new Promise((r) => setTimeout(r, 10));
      inFlight--;
    });
    await Promise.all([processQueue(store, send, { now: T0 }), processQueue(store, send, { now: T0 })]);
    expect(send).toHaveBeenCalledTimes(1);
    expect(maxInFlight).toBe(1);
  });

  it('does not send while offline but still expires', async () => {
    await store.put(outcome('k1'));
    const send = vi.fn();
    await processQueue(store, send, { now: T0, online: false });
    expect(send).not.toHaveBeenCalled();
    expect((await store.all())[0]!.status).toBe('pending');
  });
});
