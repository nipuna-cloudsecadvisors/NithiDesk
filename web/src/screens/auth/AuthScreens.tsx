import { useState, type FormEvent, type ReactNode } from 'react';
import { ApiError } from '../../api/client';
import { useAuth } from '../../auth/AuthProvider';
import { useT } from '../../i18n/I18nProvider';
import { ErrorNotice, Field, Notice } from '../../ui/components';
import { LangToggle } from '../../ui/Shell';
import { useQueue } from '../../offline/QueueProvider';

export function AuthFrame({ title, children }: { title: string; children: ReactNode }) {
  const { t } = useT();
  return (
    <div className="auth">
      <main className="auth__card" id="main">
        <div className="auth__lang">
          <LangToggle />
        </div>
        <p className="auth__brand">{t('app.name')}</p>
        <h1 className="section__title">{title}</h1>
        {children}
      </main>
    </div>
  );
}

const REASON_KEYS: Record<string, string> = {
  'auth.session_expired': 'login.reason.expired',
  logged_out: 'login.reason.loggedOut',
  network: 'login.reason.network',
};

export function LoginScreen({ reason }: { reason?: string }) {
  const { t, tl } = useT();
  const { login } = useAuth();
  const { outstanding } = useQueue();
  const [email, setEmail] = useState('');
  const [password, setPassword] = useState('');
  const [firm, setFirm] = useState('');
  const [showFirm, setShowFirm] = useState(false);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<unknown>(null);

  const submit = async (e: FormEvent) => {
    e.preventDefault();
    setBusy(true);
    setError(null);
    try {
      await login(email, password, firm);
    } catch (err) {
      setError(err);
    } finally {
      setBusy(false);
    }
  };

  const reasonKey = reason ? REASON_KEYS[reason] : undefined;
  return (
    <AuthFrame title={t('login.title')}>
      <form className="form" onSubmit={submit} noValidate>
        {reasonKey ? <Notice kind="info">{t(reasonKey)}</Notice> : null}
        {outstanding > 0 ? <Notice kind="warn">{t('login.queueWaiting', { count: outstanding })}</Notice> : null}
        <ErrorNotice error={error} />
        <Field label={tl('login.email')}>
          {(p) => (
            <input {...p} type="email" autoComplete="username" required value={email} onChange={(e) => setEmail(e.target.value)} />
          )}
        </Field>
        <Field label={tl('login.password')}>
          {(p) => (
            <input
              {...p}
              type="password"
              autoComplete="current-password"
              required
              value={password}
              onChange={(e) => setPassword(e.target.value)}
            />
          )}
        </Field>
        {showFirm ? (
          <Field label={tl('login.firm')} hint={t('login.firmHint')}>
            {(p) => <input {...p} type="text" autoComplete="organization" value={firm} onChange={(e) => setFirm(e.target.value)} />}
          </Field>
        ) : (
          <button type="button" className="btn btn--quiet btn--small" onClick={() => setShowFirm(true)}>
            {t('login.otherFirm')}
          </button>
        )}
        <button className="btn btn--primary btn--block" type="submit" disabled={busy || !email || !password}>
          {busy ? t('login.signingIn') : t('login.submit')}
        </button>
      </form>
    </AuthFrame>
  );
}

export function PinUnlockScreen() {
  const { t, tl } = useT();
  const { unlock, logout } = useAuth();
  const [pin, setPin] = useState('');
  const [error, setError] = useState<unknown>(null);
  const [busy, setBusy] = useState(false);

  const submit = async (e: FormEvent) => {
    e.preventDefault();
    setBusy(true);
    setError(null);
    try {
      await unlock(pin);
    } catch (err) {
      setPin('');
      setError(err);
      // Five wrong PINs end the session: the server then wants a full login.
      if (err instanceof ApiError && (err.code === 'auth.session_required' || err.code === 'auth.session_expired')) {
        await logout().catch(() => undefined);
      }
    } finally {
      setBusy(false);
    }
  };

  return (
    <AuthFrame title={t('pin.unlock.title')}>
      <form className="form" onSubmit={submit} noValidate>
        <p>{t('pin.unlock.body')}</p>
        <ErrorNotice error={error} />
        <Field label={tl('pin.label')}>
          {(p) => (
            <input
              {...p}
              className="pin-input"
              type="password"
              inputMode="numeric"
              autoComplete="off"
              pattern="[0-9]*"
              maxLength={8}
              value={pin}
              onChange={(e) => setPin(e.target.value.replace(/\D/g, ''))}
              autoFocus
            />
          )}
        </Field>
        <button className="btn btn--primary btn--block" type="submit" disabled={busy || pin.length < 4}>
          {t('pin.unlock.submit')}
        </button>
        <button className="btn btn--quiet" type="button" onClick={() => void logout()}>
          {t('pin.unlock.fullLogin')}
        </button>
      </form>
    </AuthFrame>
  );
}
