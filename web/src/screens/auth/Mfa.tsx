import { useEffect, useMemo, useState, type FormEvent } from 'react';
import QRCode from 'qrcode';
import { api } from '../../api/client';
import type { AuthResponse, ConfirmResponse, TOTPEnrollment } from '../../api/types';
import { useAuth } from '../../auth/AuthProvider';
import { useT } from '../../i18n/I18nProvider';
import { ErrorNotice, Field, Notice } from '../../ui/components';
import { AuthFrame } from './AuthScreens';

// Separate chunk: MFA setup pulls in the QR encoder, needed once per user.

/** Renders a QR code locally as SVG paths (no remote QR service, no inline style). */
export function QrCode({ text, label }: { text: string; label: string }) {
  const { path, size } = useMemo(() => {
    const qr = QRCode.create(text, { errorCorrectionLevel: 'M' });
    const n = qr.modules.size;
    let d = '';
    for (let r = 0; r < n; r++) {
      for (let c = 0; c < n; c++) {
        if (qr.modules.get(r, c)) d += `M${c + 4} ${r + 4}h1v1h-1z`;
      }
    }
    return { path: d, size: n + 8 };
  }, [text]);
  return (
    <svg className="qr" viewBox={`0 0 ${size} ${size}`} role="img" aria-label={label} shapeRendering="crispEdges">
      <rect width={size} height={size} fill="#fff" />
      <path d={path} fill="#000" />
    </svg>
  );
}

function CodeField({ value, onChange, label }: { value: string; onChange: (v: string) => void; label: string }) {
  return (
    <Field label={label}>
      {(p) => (
        <input
          {...p}
          className="code-input"
          inputMode="numeric"
          autoComplete="one-time-code"
          pattern="[0-9]*"
          maxLength={6}
          value={value}
          onChange={(e) => onChange(e.target.value.replace(/\D/g, ''))}
        />
      )}
    </Field>
  );
}

export function MfaScreen({ mode }: { mode: 'enroll_required' | 'verify_required' }) {
  return mode === 'enroll_required' ? <MfaEnroll /> : <MfaVerify />;
}

function MfaEnroll() {
  const { t, tl } = useT();
  const { afterMfa, logout } = useAuth();
  const [enrolment, setEnrolment] = useState<TOTPEnrollment | null>(null);
  const [code, setCode] = useState('');
  const [codes, setCodes] = useState<string[] | null>(null);
  const [csrf, setCsrf] = useState('');
  const [saved, setSaved] = useState(false);
  const [error, setError] = useState<unknown>(null);
  const [busy, setBusy] = useState(false);

  useEffect(() => {
    api
      .post<TOTPEnrollment>('/api/v1/auth/mfa/totp/enroll', undefined, { noAuthFlow: true })
      .then(setEnrolment)
      .catch(setError);
  }, []);

  const confirm = async (e: FormEvent) => {
    e.preventDefault();
    setBusy(true);
    setError(null);
    try {
      const r = await api.post<ConfirmResponse>('/api/v1/auth/mfa/totp/confirm', { code }, { noAuthFlow: true });
      api.setCsrf(r.csrf_token);
      setCsrf(r.csrf_token);
      setCodes(r.recovery_codes ?? []);
    } catch (err) {
      setError(err);
    } finally {
      setBusy(false);
    }
  };

  if (codes) {
    return (
      <AuthFrame title={t('mfa.recovery.title')}>
        <div className="stack">
          <p>{t('mfa.recovery.body')}</p>
          <ul className="recovery-codes" aria-label={t('mfa.recovery.title')}>
            {codes.map((c) => (
              <li key={c}>{c}</li>
            ))}
          </ul>
          <Notice kind="warn">{t('mfa.recovery.once')}</Notice>
          <label className="choice">
            <input type="checkbox" checked={saved} onChange={(e) => setSaved(e.target.checked)} />
            {t('mfa.recovery.saved')}
          </label>
          <button className="btn btn--primary btn--block" type="button" disabled={!saved} onClick={() => void afterMfa(csrf)}>
            {t('mfa.recovery.continue')}
          </button>
        </div>
      </AuthFrame>
    );
  }

  return (
    <AuthFrame title={t('mfa.enroll.title')}>
      <form className="form" onSubmit={confirm} noValidate>
        <p>{t('mfa.enroll.body')}</p>
        <ErrorNotice error={error} />
        {enrolment ? (
          <>
            <QrCode text={enrolment.otpauth_uri} label={t('mfa.enroll.qrLabel')} />
            <p className="small muted">{t('mfa.enroll.manual')}</p>
            <p className="secret">{enrolment.secret.replace(/(.{4})/g, '$1 ').trim()}</p>
          </>
        ) : null}
        <CodeField label={tl('mfa.code')} value={code} onChange={setCode} />
        <button className="btn btn--primary btn--block" type="submit" disabled={busy || code.length !== 6 || !enrolment}>
          {t('mfa.enroll.confirm')}
        </button>
        <button className="btn btn--quiet" type="button" onClick={() => void logout()}>
          {t('common.signOut')}
        </button>
      </form>
    </AuthFrame>
  );
}

function MfaVerify() {
  const { t, tl } = useT();
  const { afterMfa, logout } = useAuth();
  const [code, setCode] = useState('');
  const [recovery, setRecovery] = useState('');
  const [useRecovery, setUseRecovery] = useState(false);
  const [error, setError] = useState<unknown>(null);
  const [busy, setBusy] = useState(false);

  const submit = async (e: FormEvent) => {
    e.preventDefault();
    setBusy(true);
    setError(null);
    try {
      const body = useRecovery ? { recovery_code: recovery.trim() } : { code };
      const r = await api.post<AuthResponse>('/api/v1/auth/mfa/verify', body, { noAuthFlow: true });
      await afterMfa(r.csrf_token);
    } catch (err) {
      setError(err);
    } finally {
      setBusy(false);
    }
  };

  return (
    <AuthFrame title={t('mfa.verify.title')}>
      <form className="form" onSubmit={submit} noValidate>
        <p>{useRecovery ? t('mfa.verify.recoveryBody') : t('mfa.verify.body')}</p>
        <ErrorNotice error={error} />
        {useRecovery ? (
          <Field label={tl('mfa.recoveryCode')}>
            {(p) => <input {...p} type="text" autoComplete="off" value={recovery} onChange={(e) => setRecovery(e.target.value)} />}
          </Field>
        ) : (
          <CodeField label={tl('mfa.code')} value={code} onChange={setCode} />
        )}
        <button
          className="btn btn--primary btn--block"
          type="submit"
          disabled={busy || (useRecovery ? recovery.trim().length < 6 : code.length !== 6)}
        >
          {t('mfa.verify.submit')}
        </button>
        <button className="btn btn--quiet" type="button" onClick={() => setUseRecovery((x) => !x)}>
          {useRecovery ? t('mfa.verify.useApp') : t('mfa.verify.useRecovery')}
        </button>
        <button className="btn btn--quiet" type="button" onClick={() => void logout()}>
          {t('common.signOut')}
        </button>
      </form>
    </AuthFrame>
  );
}

export default MfaScreen;
