import { useState, type FormEvent } from 'react';
import { Link } from 'react-router-dom';
import { api } from '../api/client';
import type { SessionView } from '../api/types';
import { isClerk, useAuth, useMe } from '../auth/AuthProvider';
import { useT } from '../i18n/I18nProvider';
import { usePrefs } from '../i18n/Prefs';
import { useQueue } from '../offline/QueueProvider';
import { ErrorNotice, Field, Loading, Notice, Segmented, Sheet, useAsync, useToast } from '../ui/components';
import { GlobalSearch } from '../ui/Shell';
import { Stamp } from './hearing/HearingScreen';

export function LogoutButton({ className = 'btn btn--danger' }: { className?: string }) {
  const { t } = useT();
  const { logout } = useAuth();
  const { items, outstanding, clearAll } = useQueue();
  const [ask, setAsk] = useState(false);
  const doLogout = async () => {
    // The queue holds court-day data on this device: clear it on an explicit sign-out.
    await clearAll();
    await logout();
  };
  return (
    <>
      <button type="button" className={className} onClick={() => (outstanding > 0 || items.length > 0 ? setAsk(true) : void doLogout())}>
        {t('common.signOut')}
      </button>
      <Sheet
        open={ask}
        title={t('logout.pendingTitle')}
        onClose={() => setAsk(false)}
        footer={
          <>
            <Link className="btn" to="/outbox" onClick={() => setAsk(false)}>
              {t('logout.review')}
            </Link>
            <button type="button" className="btn btn--danger" onClick={() => void doLogout()}>
              {t('logout.discardAndSignOut')}
            </button>
          </>
        }
      >
        <Notice kind="warn">{t('logout.pendingBody', { count: outstanding })}</Notice>
      </Sheet>
    </>
  );
}

export function SettingsScreen() {
  const { t, tl } = useT();
  const me = useMe();
  const { locale, labelMode, setLocale, setLabelMode } = usePrefs();
  const toast = useToast();
  const sessions = useAsync((signal) => api.get<{ items: SessionView[] | null }>('/api/v1/me/sessions', { signal }).then((r) => r.items ?? []), []);
  const [prefError, setPrefError] = useState<unknown>(null);

  const wrap = (p: Promise<void>) => {
    setPrefError(null);
    p.then(() => toast('ok', t('common.saved'))).catch(setPrefError);
  };

  return (
    <div className="stack">
      <h1>{tl('nav.settings')}</h1>
      <ErrorNotice error={prefError} />
      <section className="panel stack">
        <h2>{tl('settings.language')}</h2>
        <Segmented
          label={t('settings.language')}
          value={locale}
          onChange={(l) => wrap(setLocale(l))}
          options={[
            { value: 'en', label: t('lang.en') },
            { value: 'si', label: t('lang.si') },
          ]}
        />
        <h3>{tl('settings.labelMode')}</h3>
        <p className="small muted">{t('settings.labelModeHint')}</p>
        <Segmented
          label={t('settings.labelMode')}
          value={labelMode}
          onChange={(m) => wrap(setLabelMode(m))}
          options={[
            { value: 'single', label: t('settings.labelSingle') },
            { value: 'dual', label: t('settings.labelDual') },
          ]}
        />
      </section>

      {me.client_kind === 'mobile' ? <PinSection hasPin={me.has_pin} /> : null}
      <PasswordSection />

      <section className="panel stack">
        <h2>{tl('settings.sessions')}</h2>
        {sessions.loading && !sessions.data ? <Loading /> : null}
        <ErrorNotice error={sessions.error} />
        <ul className="list">
          {(sessions.data ?? []).map((s) => (
            <li key={s.id} className="row">
              <span className="grow">
                <strong>{s.device_label ?? t(`clientKind.${s.client_kind}`)}</strong>
                {s.current ? <span className="tag tag--ok">{` ${t('settings.thisDevice')}`}</span> : null}
                <br />
                <span className="small muted">
                  {t('settings.lastSeen')} <Stamp at={s.last_seen_at} />
                </span>
              </span>
              {!s.current ? (
                <button
                  type="button"
                  className="btn btn--small btn--danger"
                  onClick={() =>
                    void api
                      .del(`/api/v1/me/sessions/${s.id}`)
                      .then(() => {
                        toast('ok', t('settings.revoked'));
                        sessions.reload();
                      })
                      .catch(setPrefError)
                  }
                >
                  {t('settings.revoke')}
                </button>
              ) : null}
            </li>
          ))}
        </ul>
      </section>

      <section className="panel">
        <LogoutButton />
      </section>
    </div>
  );
}

function PinSection({ hasPin }: { hasPin: boolean }) {
  const { t, tl, fieldError } = useT();
  const { refreshMe } = useAuth();
  const toast = useToast();
  const [pin, setPin] = useState('');
  const [again, setAgain] = useState('');
  const [error, setError] = useState<unknown>(null);
  const mismatch = again.length > 0 && pin !== again;
  const submit = async (e: FormEvent) => {
    e.preventDefault();
    setError(null);
    try {
      await api.post('/api/v1/auth/pin', { pin });
      setPin('');
      setAgain('');
      toast('ok', t('pin.saved'));
      await refreshMe();
    } catch (err) {
      setError(err);
    }
  };
  return (
    <section className="panel">
      <form className="form" onSubmit={submit} noValidate>
        <h2>{tl('pin.title')}</h2>
        <p className="small muted">{hasPin ? t('pin.hasPin') : t('pin.explain')}</p>
        <ErrorNotice error={error} />
        <div className="grid-2">
          <Field label={tl('pin.label')} error={fieldError(error, 'pin')}>
            {(p) => (
              <input {...p} type="password" inputMode="numeric" maxLength={8} autoComplete="off" value={pin} onChange={(e) => setPin(e.target.value.replace(/\D/g, ''))} />
            )}
          </Field>
          <Field label={tl('pin.again')} error={mismatch ? t('pin.mismatch') : undefined}>
            {(p) => (
              <input {...p} type="password" inputMode="numeric" maxLength={8} autoComplete="off" value={again} onChange={(e) => setAgain(e.target.value.replace(/\D/g, ''))} />
            )}
          </Field>
        </div>
        <div>
          <button type="submit" className="btn btn--primary" disabled={pin.length < 4 || pin !== again}>
            {t('pin.save')}
          </button>
        </div>
      </form>
    </section>
  );
}

function PasswordSection() {
  const { t, tl, fieldError } = useT();
  const toast = useToast();
  const [current, setCurrent] = useState('');
  const [next, setNext] = useState('');
  const [error, setError] = useState<unknown>(null);
  const submit = async (e: FormEvent) => {
    e.preventDefault();
    setError(null);
    try {
      await api.post('/api/v1/auth/password', { current_password: current, new_password: next });
      setCurrent('');
      setNext('');
      toast('ok', t('password.changed'));
    } catch (err) {
      setError(err);
    }
  };
  return (
    <section className="panel">
      <form className="form" onSubmit={submit} noValidate>
        <h2>{tl('password.title')}</h2>
        <ErrorNotice error={error} />
        <Field label={tl('password.current')}>
          {(p) => <input {...p} type="password" autoComplete="current-password" value={current} onChange={(e) => setCurrent(e.target.value)} />}
        </Field>
        <Field label={tl('password.new')} hint={t('password.policy')} error={fieldError(error, 'new_password')}>
          {(p) => <input {...p} type="password" autoComplete="new-password" value={next} onChange={(e) => setNext(e.target.value)} />}
        </Field>
        <div>
          <button type="submit" className="btn btn--primary" disabled={!current || next.length < 12}>
            {t('password.save')}
          </button>
        </div>
      </form>
    </section>
  );
}

/** Mobile "More" tab (doc 12 §6.3). */
export function MoreScreen() {
  const { tl } = useT();
  const me = useMe();
  const can = (c: string) => me.capabilities.includes(c as never);
  const links: [string, string][] = [
    ['/week', tl('nav.diary')],
    ['/matters', tl('nav.myMatters')],
    ['/brief', tl('nav.brief')],
    ['/outcomes-pending', tl('nav.outcomesPending')],
  ];
  if (!isClerk(me)) links.unshift(['/verification', tl('nav.verification')]);
  if (isClerk(me)) links.unshift(['/today', tl('nav.today')]);
  if (can('import.run')) links.push(['/imports', tl('nav.imports')]);
  if (can('user.manage')) links.push(['/admin', tl('nav.admin')]);
  links.push(['/settings', tl('nav.settings')]);
  return (
    <div className="stack">
      <h1>{tl('nav.more')}</h1>
      <ul className="list">
        {links.map(([to, label]) => (
          <li key={to}>
            <Link className="list__link" to={to}>
              <span className="list__title">{label}</span>
            </Link>
          </li>
        ))}
      </ul>
      <LogoutButton className="btn btn--danger btn--block" />
    </div>
  );
}

export function SearchScreen() {
  const { tl } = useT();
  return (
    <div className="stack">
      <h1>{tl('nav.search')}</h1>
      <GlobalSearch autoFocus />
    </div>
  );
}
