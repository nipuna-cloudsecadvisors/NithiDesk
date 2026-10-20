import { describe, expect, it, vi } from 'vitest';
import { ApiClient, ApiError, isRetryable, NetworkError, parseErrorBody, type AuthEvent } from './client';

type Call = { url: string; init: RequestInit };

function fakeFetch(responses: (Response | Error)[]) {
  const calls: Call[] = [];
  const fn = vi.fn(async (url: string, init: RequestInit) => {
    calls.push({ url, init });
    const next = responses.shift();
    if (!next) throw new Error('no more responses');
    if (next instanceof Error) throw next;
    return next;
  });
  return { fn: fn as unknown as typeof fetch, calls };
}

const json = (status: number, body: unknown, headers: Record<string, string> = {}) =>
  new Response(JSON.stringify(body), { status, headers: { 'Content-Type': 'application/json', ...headers } });

const err = (status: number, code: string, extra: Record<string, unknown> = {}) =>
  json(status, { error: { code, request_id: 'req-123', ...extra } });

const header = (c: Call, name: string) => (c.init.headers as Record<string, string>)[name];

describe('ApiClient headers', () => {
  it('sends the CSRF token on writes but not on reads', async () => {
    const f = fakeFetch([json(200, { ok: 1 }), json(200, { ok: 2 })]);
    const c = new ApiClient({ fetch: f.fn });
    c.setCsrf('tok');
    await c.get('/api/v1/me');
    await c.post('/api/v1/hearings/x/notes', { body: 'n' });
    expect(header(f.calls[0]!, 'X-CSRF-Token')).toBeUndefined();
    expect(header(f.calls[1]!, 'X-CSRF-Token')).toBe('tok');
    expect(header(f.calls[1]!, 'Content-Type')).toBe('application/json');
    expect(f.calls[1]!.init.credentials).toBe('same-origin');
  });

  it('quotes If-Match and passes the Idempotency-Key', async () => {
    const f = fakeFetch([json(200, {}), json(200, {})]);
    const c = new ApiClient({ fetch: f.fn });
    await c.patch('/api/v1/hearings/x', { notes: 'a' }, 7);
    await c.put('/api/v1/hearings/x/outcome', {}, { idempotencyKey: 'key-12345678' });
    expect(header(f.calls[0]!, 'If-Match')).toBe('"7"');
    expect(f.calls[0]!.init.method).toBe('PATCH');
    expect(header(f.calls[1]!, 'Idempotency-Key')).toBe('key-12345678');
  });

  it('builds query strings and skips empty values', async () => {
    const f = fakeFetch([json(200, {})]);
    const c = new ApiClient({ fetch: f.fn });
    await c.get('/api/v1/diary/week', { query: { start: '2026-10-20', lawyer: '', court: undefined, days: 7 } });
    expect(f.calls[0]!.url).toBe('/api/v1/diary/week?start=2026-10-20&days=7');
  });

  it('returns undefined for 204', async () => {
    const f = fakeFetch([new Response(null, { status: 204 })]);
    const c = new ApiClient({ fetch: f.fn });
    await expect(c.post('/api/v1/auth/logout')).resolves.toBeUndefined();
  });
});

describe('error envelope', () => {
  it('parses code, request id, params and field errors', async () => {
    const f = fakeFetch([
      err(422, 'validation.failed', { params: { a: 1 }, fields: [{ field: 'next_hearing.date', code: 'hearing.next_date_not_after' }] }),
    ]);
    const c = new ApiClient({ fetch: f.fn });
    const e = (await c.post('/x', {}).catch((x) => x)) as ApiError;
    expect(e).toBeInstanceOf(ApiError);
    expect(e.status).toBe(422);
    expect(e.code).toBe('validation.failed');
    expect(e.requestId).toBe('req-123');
    expect(e.params).toEqual({ a: 1 });
    expect(e.fieldCode('next_hearing.date')).toBe('hearing.next_date_not_after');
  });

  it('handles the bare recover-middleware body and non-JSON bodies', () => {
    expect(parseErrorBody(500, { error: 'internal_error' }).code).toBe('internal_error');
    expect(parseErrorBody(502, null).code).toBe('internal_error');
    expect(parseErrorBody(418, 'teapot').code).toBe('unknown');
  });

  it('turns a failed fetch into NetworkError', async () => {
    const f = fakeFetch([new TypeError('Failed to fetch')]);
    const c = new ApiClient({ fetch: f.fn });
    await expect(c.get('/x')).rejects.toBeInstanceOf(NetworkError);
  });

  it('classifies retryable failures', () => {
    expect(isRetryable(new NetworkError())).toBe(true);
    expect(isRetryable(new ApiError(503, 'internal_error'))).toBe(true);
    expect(isRetryable(new ApiError(429, 'rate_limited'))).toBe(true);
    expect(isRetryable(new ApiError(409, 'idempotency.in_progress'))).toBe(true);
    expect(isRetryable(new ApiError(409, 'hearing.outcome_already_recorded'))).toBe(false);
    expect(isRetryable(new ApiError(412, 'precondition.version_mismatch'))).toBe(false);
    expect(isRetryable(new ApiError(422, 'validation.failed'))).toBe(false);
  });
});

describe('401 flows', () => {
  const run = async (res: Response) => {
    const f = fakeFetch([res]);
    const c = new ApiClient({ fetch: f.fn });
    const events: AuthEvent[] = [];
    c.onAuth((e) => events.push(e));
    await c.get('/x').catch(() => undefined);
    return events;
  };

  it('session required / expired → login', async () => {
    expect(await run(err(401, 'auth.session_expired'))).toEqual([{ kind: 'login', code: 'auth.session_expired' }]);
    expect(await run(err(401, 'auth.session_required'))).toEqual([{ kind: 'login', code: 'auth.session_required' }]);
  });

  it('session locked → PIN unlock', async () => {
    expect(await run(err(401, 'auth.session_locked'))).toEqual([{ kind: 'locked' }]);
  });

  it('mfa required carries the state', async () => {
    expect(await run(err(401, 'auth.mfa_required', { params: { state: 'enroll_required' } }))).toEqual([
      { kind: 'mfa', state: 'enroll_required' },
    ]);
  });

  it('wrong password does not trigger a flow', async () => {
    expect(await run(err(401, 'auth.invalid_credentials'))).toEqual([]);
  });

  it('noAuthFlow suppresses events', async () => {
    const f = fakeFetch([err(401, 'auth.session_expired')]);
    const c = new ApiClient({ fetch: f.fn });
    const events: AuthEvent[] = [];
    c.onAuth((e) => events.push(e));
    await c.get('/x', { noAuthFlow: true }).catch(() => undefined);
    expect(events).toEqual([]);
  });

  it('step-up: asks, then retries the original request once', async () => {
    const f = fakeFetch([err(401, 'auth.step_up_required'), new Response(null, { status: 204 })]);
    const c = new ApiClient({ fetch: f.fn });
    c.setCsrf('tok');
    const handler = vi.fn(async () => true);
    c.setStepUpHandler(handler);
    await expect(c.post('/api/v1/admin/users/u/disable')).resolves.toBeUndefined();
    expect(handler).toHaveBeenCalledTimes(1);
    expect(f.calls).toHaveLength(2);
    expect(f.calls[1]!.url).toBe('/api/v1/admin/users/u/disable');
    expect(header(f.calls[1]!, 'X-CSRF-Token')).toBe('tok');
  });

  it('step-up cancelled: the original error is thrown', async () => {
    const f = fakeFetch([err(401, 'auth.step_up_required')]);
    const c = new ApiClient({ fetch: f.fn });
    c.setStepUpHandler(async () => false);
    const e = (await c.post('/x').catch((x) => x)) as ApiError;
    expect(e.code).toBe('auth.step_up_required');
    expect(f.calls).toHaveLength(1);
  });
});
