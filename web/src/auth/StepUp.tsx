import { useEffect, useRef, useState, type FormEvent } from 'react';
import { api } from '../api/client';
import { useT } from '../i18n/I18nProvider';
import { ErrorNotice, Field, Sheet } from '../ui/components';

/**
 * Answers auth.step_up_required: asks for password + authenticator code,
 * calls POST /auth/step-up, and lets the client retry the original request.
 */
export function StepUpHost() {
  const { t, tl } = useT();
  const [open, setOpen] = useState(false);
  const [password, setPassword] = useState('');
  const [code, setCode] = useState('');
  const [error, setError] = useState<unknown>(null);
  const [busy, setBusy] = useState(false);
  const resolver = useRef<((ok: boolean) => void) | null>(null);

  useEffect(() => {
    api.setStepUpHandler(
      () =>
        new Promise<boolean>((resolve) => {
          resolver.current?.(false);
          resolver.current = resolve;
          setPassword('');
          setCode('');
          setError(null);
          setOpen(true);
        }),
    );
    return () => api.setStepUpHandler(null);
  }, []);

  const finish = (ok: boolean) => {
    setOpen(false);
    setPassword('');
    setCode('');
    const r = resolver.current;
    resolver.current = null;
    r?.(ok);
  };

  const submit = async (e: FormEvent) => {
    e.preventDefault();
    setBusy(true);
    setError(null);
    try {
      await api.post('/api/v1/auth/step-up', { password, code }, { noAuthFlow: true });
      finish(true);
    } catch (err) {
      setError(err);
    } finally {
      setBusy(false);
    }
  };

  return (
    <Sheet open={open} title={t('stepUp.title')} onClose={() => finish(false)}>
      <form className="form" onSubmit={submit} noValidate>
        <p>{t('stepUp.body')}</p>
        <ErrorNotice error={error} />
        <Field label={tl('login.password')}>
          {(p) => (
            <input {...p} type="password" autoComplete="current-password" value={password} onChange={(e) => setPassword(e.target.value)} />
          )}
        </Field>
        <Field label={tl('mfa.code')}>
          {(p) => (
            <input
              {...p}
              className="code-input"
              inputMode="numeric"
              autoComplete="one-time-code"
              maxLength={6}
              value={code}
              onChange={(e) => setCode(e.target.value.replace(/\D/g, ''))}
            />
          )}
        </Field>
        <div className="row row--end">
          <button type="button" className="btn" onClick={() => finish(false)}>
            {t('common.cancel')}
          </button>
          <button type="submit" className="btn btn--primary" disabled={busy || !password || code.length !== 6}>
            {t('stepUp.submit')}
          </button>
        </div>
      </form>
    </Sheet>
  );
}
