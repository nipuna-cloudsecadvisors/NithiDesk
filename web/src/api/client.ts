// The one module that talks to the Go API (internal/httpapi). It owns the
// CSRF token, the error envelope, Idempotency-Key and If-Match headers, and
// the 401 flows (login, PIN unlock, MFA, step-up).

export interface FieldErrorBody {
  field: string;
  code: string;
  params?: Record<string, unknown>;
}

export interface ErrorEnvelope {
  error: {
    code: string;
    request_id: string;
    params?: Record<string, unknown>;
    fields?: FieldErrorBody[];
  };
}

export interface Warning {
  code: string;
  params?: Record<string, unknown>;
}

export interface WithWarnings<T> {
  data: T;
  warnings: Warning[];
}

/** An error response from the API, carrying its stable code. */
export class ApiError extends Error {
  readonly status: number;
  readonly code: string;
  readonly requestId: string;
  readonly params: Record<string, unknown>;
  readonly fields: FieldErrorBody[];

  constructor(status: number, code: string, requestId = '', params: Record<string, unknown> = {}, fields: FieldErrorBody[] = []) {
    super(code);
    this.name = 'ApiError';
    this.status = status;
    this.code = code;
    this.requestId = requestId;
    this.params = params;
    this.fields = fields;
  }

  fieldCode(field: string): string | undefined {
    return this.fields.find((f) => f.field === field)?.code;
  }
}

/** The request never reached the API (offline, DNS, aborted connection). */
export class NetworkError extends Error {
  constructor(cause?: unknown) {
    super('network');
    this.name = 'NetworkError';
    if (cause !== undefined) (this as { cause?: unknown }).cause = cause;
  }
}

export type AuthEvent =
  | { kind: 'login'; code: string }
  | { kind: 'locked' }
  | { kind: 'mfa'; state: string };

type StepUpHandler = () => Promise<boolean>;

export interface RequestOptions {
  query?: Record<string, string | number | boolean | undefined | null>;
  body?: unknown;
  form?: FormData;
  ifMatch?: number | string;
  idempotencyKey?: string;
  signal?: AbortSignal;
  /** Do not run the global 401 flows (used by login/MFA/unlock forms and the offline queue). */
  noAuthFlow?: boolean;
  /** Return the raw Response (file downloads). */
  raw?: boolean;
}

export interface ClientConfig {
  base: string;
  fetch: typeof fetch;
}

const AUTH_LOGIN_CODES = new Set(['auth.session_required', 'auth.session_expired']);

export function parseErrorBody(status: number, body: unknown): ApiError {
  const env = body as Partial<ErrorEnvelope> | null;
  const e = env && typeof env === 'object' ? env.error : undefined;
  if (e && typeof e === 'object' && typeof e.code === 'string') {
    return new ApiError(status, e.code, e.request_id ?? '', e.params ?? {}, Array.isArray(e.fields) ? e.fields : []);
  }
  // The recover middleware answers {"error":"internal_error"}.
  const bare = (body as { error?: unknown } | null)?.error;
  if (typeof bare === 'string') return new ApiError(status, bare);
  return new ApiError(status, status >= 500 ? 'internal_error' : 'unknown');
}

export class ApiClient {
  private csrf = '';
  private listeners = new Set<(e: AuthEvent) => void>();
  private stepUp: StepUpHandler | null = null;
  private cfg: ClientConfig;

  constructor(cfg: Partial<ClientConfig> = {}) {
    this.cfg = { base: cfg.base ?? '', fetch: cfg.fetch ?? ((...a) => globalThis.fetch(...a)) };
  }

  setCsrf(token: string | undefined | null): void {
    this.csrf = token ?? '';
  }

  get csrfToken(): string {
    return this.csrf;
  }

  onAuth(fn: (e: AuthEvent) => void): () => void {
    this.listeners.add(fn);
    return () => this.listeners.delete(fn);
  }

  setStepUpHandler(fn: StepUpHandler | null): void {
    this.stepUp = fn;
  }

  private emit(e: AuthEvent): void {
    for (const l of this.listeners) l(e);
  }

  url(path: string, query?: RequestOptions['query']): string {
    let u = this.cfg.base + path;
    if (query) {
      const qs = new URLSearchParams();
      for (const [k, v] of Object.entries(query)) {
        if (v !== undefined && v !== null && v !== '') qs.set(k, String(v));
      }
      const s = qs.toString();
      if (s) u += (u.includes('?') ? '&' : '?') + s;
    }
    return u;
  }

  async request<T>(method: string, path: string, opts: RequestOptions = {}): Promise<T> {
    try {
      return await this.once<T>(method, path, opts);
    } catch (err) {
      if (!(err instanceof ApiError) || err.status !== 401 || opts.noAuthFlow) throw err;
      if (err.code === 'auth.step_up_required' && this.stepUp) {
        const ok = await this.stepUp();
        if (ok) return this.once<T>(method, path, opts);
        throw err;
      }
      if (AUTH_LOGIN_CODES.has(err.code)) this.emit({ kind: 'login', code: err.code });
      else if (err.code === 'auth.session_locked') this.emit({ kind: 'locked' });
      else if (err.code === 'auth.mfa_required') this.emit({ kind: 'mfa', state: String(err.params.state ?? '') });
      throw err;
    }
  }

  private async once<T>(method: string, path: string, opts: RequestOptions): Promise<T> {
    const headers: Record<string, string> = { Accept: 'application/json' };
    let body: BodyInit | undefined;
    if (opts.form) {
      body = opts.form;
    } else if (opts.body !== undefined) {
      headers['Content-Type'] = 'application/json';
      body = JSON.stringify(opts.body);
    }
    if (method !== 'GET' && method !== 'HEAD' && this.csrf) headers['X-CSRF-Token'] = this.csrf;
    if (opts.ifMatch !== undefined) headers['If-Match'] = `"${opts.ifMatch}"`;
    if (opts.idempotencyKey) headers['Idempotency-Key'] = opts.idempotencyKey;

    let res: Response;
    try {
      res = await this.cfg.fetch(this.url(path, opts.query), {
        method,
        headers,
        body,
        credentials: 'same-origin',
        signal: opts.signal,
        cache: 'no-store',
      });
    } catch (e) {
      if (e instanceof DOMException && e.name === 'AbortError') throw e;
      throw new NetworkError(e);
    }
    if (opts.raw && res.ok) return res as unknown as T;
    if (res.status === 204) return undefined as T;
    const text = await res.text();
    let parsed: unknown = null;
    if (text) {
      try {
        parsed = JSON.parse(text);
      } catch {
        parsed = null;
      }
    }
    if (!res.ok) throw parseErrorBody(res.status, parsed);
    return parsed as T;
  }

  get<T>(path: string, opts?: RequestOptions) {
    return this.request<T>('GET', path, opts);
  }
  post<T>(path: string, body?: unknown, opts: RequestOptions = {}) {
    return this.request<T>('POST', path, { ...opts, body });
  }
  put<T>(path: string, body?: unknown, opts: RequestOptions = {}) {
    return this.request<T>('PUT', path, { ...opts, body });
  }
  patch<T>(path: string, body: unknown, ifMatch: number, opts: RequestOptions = {}) {
    return this.request<T>('PATCH', path, { ...opts, body, ifMatch });
  }
  del<T>(path: string, body?: unknown, opts: RequestOptions = {}) {
    return this.request<T>('DELETE', path, { ...opts, body });
  }
}

export const api = new ApiClient();

/** Whether a failure is worth retrying later without user action. */
export function isRetryable(err: unknown): boolean {
  if (err instanceof NetworkError) return true;
  if (err instanceof ApiError) {
    if (err.status >= 500 || err.status === 429 || err.status === 408) return true;
    if (err.code === 'idempotency.in_progress') return true;
  }
  return false;
}
