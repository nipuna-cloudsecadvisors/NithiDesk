import {
  createContext,
  useCallback,
  useContext,
  useEffect,
  useId,
  useRef,
  useState,
  type ReactNode,
} from 'react';
import { useT } from '../i18n/I18nProvider';
import type { Warning } from '../api/client';
import type { VerificationState } from '../api/types';

// ---------------------------------------------------------------------------
// Status
// ---------------------------------------------------------------------------

const V_ICON: Record<VerificationState, string> = { verified: '✓', unverified: '?', conflict: '⚠' };

/** Verification is always icon + words, never colour alone (doc 06 §4.2). */
export function VerificationBadge({ state, overdue }: { state: VerificationState; overdue?: boolean }) {
  const { tl, t } = useT();
  return (
    <span className="row">
      <span className={`vstate vstate--${state}`}>
        <span className="vstate__icon" aria-hidden="true">
          {V_ICON[state] ?? '?'}
        </span>
        {tl(`verification.${state}`)}
      </span>
      {overdue && state !== 'verified' ? <span className="tag tag--wait">{t('verification.overdue')}</span> : null}
    </span>
  );
}

export function StatusTag({ kind, children }: { kind?: 'ok' | 'wait' | 'bad'; children: ReactNode }) {
  return <span className={kind ? `tag tag--${kind}` : 'tag'}>{children}</span>;
}

// ---------------------------------------------------------------------------
// Notices
// ---------------------------------------------------------------------------

type NoticeKind = 'info' | 'warn' | 'error' | 'ok';
const N_ICON: Record<NoticeKind, string> = { info: 'i', warn: '!', error: '⚠', ok: '✓' };

export function Notice({ kind = 'info', children, role }: { kind?: NoticeKind; children: ReactNode; role?: 'alert' | 'status' }) {
  return (
    <div className={`notice notice--${kind}`} role={role ?? (kind === 'error' ? 'alert' : 'status')}>
      <span className="notice__icon" aria-hidden="true">
        {N_ICON[kind]}
      </span>
      <div className="grow">{children}</div>
    </div>
  );
}

export function ErrorNotice({ error }: { error: unknown }) {
  const { errorText } = useT();
  if (!error) return null;
  return <Notice kind="error">{errorText(error)}</Notice>;
}

export function Warnings({ warnings, names }: { warnings: Warning[] | null | undefined; names?: Record<string, string> }) {
  const { warningText } = useT();
  if (!warnings || warnings.length === 0) return null;
  return (
    <div className="stack stack--tight">
      {warnings.map((w, i) => (
        <Notice key={`${w.code}-${i}`} kind="warn">
          {warningText(w, names)}
        </Notice>
      ))}
    </div>
  );
}

export function Loading() {
  const { t } = useT();
  return (
    <p className="loading" role="status">
      {t('common.loading')}
    </p>
  );
}

export function Empty({ children }: { children: ReactNode }) {
  return <div className="empty">{children}</div>;
}

// ---------------------------------------------------------------------------
// Form fields
// ---------------------------------------------------------------------------

export function Field({
  label,
  hint,
  error,
  children,
}: {
  label: string;
  hint?: string;
  error?: string;
  children: (props: { id: string; 'aria-invalid'?: boolean; 'aria-describedby'?: string }) => ReactNode;
}) {
  const id = useId();
  const hintId = hint ? `${id}-hint` : undefined;
  const errId = error ? `${id}-err` : undefined;
  const describedBy = [hintId, errId].filter(Boolean).join(' ') || undefined;
  return (
    <div className="field">
      <label htmlFor={id}>{label}</label>
      {hint ? (
        <span className="field__hint" id={hintId}>
          {hint}
        </span>
      ) : null}
      {children({ id, 'aria-invalid': error ? true : undefined, 'aria-describedby': describedBy })}
      {error ? (
        <span className="field__error" id={errId}>
          {error}
        </span>
      ) : null}
    </div>
  );
}

// ---------------------------------------------------------------------------
// Dialog (native <dialog>: focus trap, Esc, backdrop)
// ---------------------------------------------------------------------------

export function Sheet({
  open,
  title,
  onClose,
  children,
  footer,
}: {
  open: boolean;
  title: string;
  onClose: () => void;
  children: ReactNode;
  footer?: ReactNode;
}) {
  const ref = useRef<HTMLDialogElement>(null);
  const { t } = useT();
  const titleId = useId();
  useEffect(() => {
    const d = ref.current;
    if (!d) return;
    if (open && !d.open) {
      if (typeof d.showModal === 'function') d.showModal();
      else d.setAttribute('open', '');
    } else if (!open && d.open) {
      d.close();
    }
  }, [open]);
  return (
    <dialog
      ref={ref}
      className="sheet"
      aria-labelledby={titleId}
      onCancel={(e) => {
        e.preventDefault();
        onClose();
      }}
    >
      {open ? (
        <>
          <div className="sheet__head">
            <h2 id={titleId}>{title}</h2>
            <button type="button" className="btn btn--quiet btn--small" onClick={onClose}>
              {t('common.close')}
            </button>
          </div>
          <div className="sheet__body">{children}</div>
          {footer ? <div className="sheet__foot">{footer}</div> : null}
        </>
      ) : null}
    </dialog>
  );
}

// ---------------------------------------------------------------------------
// Toasts
// ---------------------------------------------------------------------------

interface Toast {
  id: number;
  kind: NoticeKind;
  text: string;
}

const ToastCtx = createContext<(kind: NoticeKind, text: string) => void>(() => undefined);

export function ToastProvider({ children }: { children: ReactNode }) {
  const [toasts, setToasts] = useState<Toast[]>([]);
  const seq = useRef(0);
  const push = useCallback((kind: NoticeKind, text: string) => {
    const id = ++seq.current;
    setToasts((ts) => [...ts.slice(-3), { id, kind, text }]);
    window.setTimeout(() => setToasts((ts) => ts.filter((x) => x.id !== id)), kind === 'error' ? 9000 : 5000);
  }, []);
  return (
    <ToastCtx.Provider value={push}>
      {children}
      <div className="toasts" aria-live="polite">
        {toasts.map((x) => (
          <Notice key={x.id} kind={x.kind}>
            {x.text}
          </Notice>
        ))}
      </div>
    </ToastCtx.Provider>
  );
}

export function useToast() {
  return useContext(ToastCtx);
}

// ---------------------------------------------------------------------------
// Data loading
// ---------------------------------------------------------------------------

export interface Async<T> {
  data: T | undefined;
  error: unknown;
  loading: boolean;
  reload: () => void;
  setData: (d: T) => void;
}

export function useAsync<T>(fn: (signal: AbortSignal) => Promise<T>, deps: unknown[]): Async<T> {
  const [data, setData] = useState<T>();
  const [error, setError] = useState<unknown>(null);
  const [loading, setLoading] = useState(true);
  const [tick, setTick] = useState(0);
  useEffect(() => {
    const ctl = new AbortController();
    setLoading(true);
    setError(null);
    fn(ctl.signal)
      .then((d) => {
        if (!ctl.signal.aborted) setData(d);
      })
      .catch((e) => {
        if (!ctl.signal.aborted && !(e instanceof DOMException && e.name === 'AbortError')) setError(e);
      })
      .finally(() => {
        if (!ctl.signal.aborted) setLoading(false);
      });
    return () => ctl.abort();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [...deps, tick]);
  return { data, error, loading, reload: () => setTick((x) => x + 1), setData };
}

/** Segmented control. */
export function Segmented<V extends string>({
  label,
  value,
  options,
  onChange,
}: {
  label: string;
  value: V;
  options: { value: V; label: string }[];
  onChange: (v: V) => void;
}) {
  return (
    <div className="seg" role="group" aria-label={label}>
      {options.map((o) => (
        <button key={o.value} type="button" aria-pressed={o.value === value} onClick={() => onChange(o.value)}>
          {o.label}
        </button>
      ))}
    </div>
  );
}
