import { useState, type FormEvent } from 'react';
import { Link, useNavigate, useParams } from 'react-router-dom';
import { api } from '../api/client';
import { clearReferenceCache } from '../api/reference';
import type { UserView } from '../api/types';
import { useMe } from '../auth/AuthProvider';
import { useT } from '../i18n/I18nProvider';
import { ErrorNotice, Field, Loading, Notice, useAsync, useToast } from '../ui/components';
import { Stamp } from './hearing/HearingScreen';

export const ROLE_CODES = [
  'managing_partner',
  'partner',
  'senior_lawyer',
  'junior_lawyer',
  'legal_assistant',
  'court_clerk',
  'system_administrator',
] as const;

export function AdminUsersScreen() {
  const { t, tl, name, otherName } = useT();
  const users = useAsync((signal) => api.get<{ items: UserView[] | null }>('/api/v1/users', { signal }).then((r) => r.items ?? []), []);
  return (
    <>
      <div className="page-head">
        <div className="page-head__title">
          <h1>{tl('admin.users')}</h1>
        </div>
        <Link className="btn btn--primary" to="/admin/users/new">
          {t('admin.newUser')}
        </Link>
      </div>
      {users.loading && !users.data ? <Loading /> : null}
      <ErrorNotice error={users.error} />
      <ul className="list">
        {(users.data ?? []).map((u) => (
          <li key={u.id}>
            <Link className="list__link" to={`/admin/users/${u.id}`}>
              <span className="list__title">{name(u.display_name)}</span>
              {otherName(u.display_name) ? <span className="muted">{` / ${otherName(u.display_name)}`}</span> : null}
            </Link>
            <p className="row small">
              {u.email ? <span>{u.email}</span> : null}
              {u.roles.map((r) => (
                <span key={r} className="tag">
                  {t(`role.${r}`)}
                </span>
              ))}
              <span className={u.status === 'disabled' ? 'tag tag--bad' : 'tag tag--ok'}>{t(`userStatus.${u.status}`)}</span>
              {!u.mfa_enrolled ? <span className="tag tag--wait">{t('admin.mfaNotEnrolled')}</span> : null}
            </p>
          </li>
        ))}
      </ul>
    </>
  );
}

function RoleChoices({ value, onChange }: { value: string[]; onChange: (v: string[]) => void }) {
  const { t, tl } = useT();
  return (
    <fieldset className="field">
      <legend>{tl('admin.roles')}</legend>
      <div className="choice-list">
        {ROLE_CODES.map((r) => (
          <label className="choice" key={r}>
            <input
              type="checkbox"
              checked={value.includes(r)}
              onChange={(e) => onChange(e.target.checked ? [...value, r] : value.filter((x) => x !== r))}
            />
            {t(`role.${r}`)}
          </label>
        ))}
      </div>
    </fieldset>
  );
}

export function AdminCreateUserScreen() {
  const { t, tl, fieldError } = useT();
  const navigate = useNavigate();
  const [email, setEmail] = useState('');
  const [nameEn, setNameEn] = useState('');
  const [nameSi, setNameSi] = useState('');
  const [locale, setLocale] = useState<'en' | 'si'>('si');
  const [password, setPassword] = useState('');
  const [roles, setRoles] = useState<string[]>([]);
  const [error, setError] = useState<unknown>(null);
  const [busy, setBusy] = useState(false);

  const submit = async (e: FormEvent) => {
    e.preventDefault();
    setBusy(true);
    setError(null);
    try {
      const u = await api.post<UserView>('/api/v1/admin/users', {
        email: email.trim(),
        display_name_en: nameEn.trim(),
        display_name_si: nameSi.trim(),
        preferred_locale: locale,
        initial_password: password,
        roles,
      });
      clearReferenceCache();
      navigate(`/admin/users/${u.id}`, { replace: true });
    } catch (err) {
      setError(err);
    } finally {
      setBusy(false);
    }
  };

  return (
    <form className="form" onSubmit={submit} noValidate>
      <h1>{t('admin.newUser')}</h1>
      <ErrorNotice error={error} />
      <Field label={tl('login.email')} error={fieldError(error, 'email')}>
        {(p) => <input {...p} type="email" autoComplete="off" required value={email} onChange={(e) => setEmail(e.target.value)} />}
      </Field>
      <div className="grid-2">
        <Field label={tl('admin.nameEn')} error={fieldError(error, 'display_name_en')}>
          {(p) => <input {...p} type="text" lang="en" required value={nameEn} onChange={(e) => setNameEn(e.target.value)} />}
        </Field>
        <Field label={tl('admin.nameSi')}>
          {(p) => <input {...p} type="text" lang="si" value={nameSi} onChange={(e) => setNameSi(e.target.value)} />}
        </Field>
      </div>
      <Field label={tl('settings.language')}>
        {(p) => (
          <select {...p} value={locale} onChange={(e) => setLocale(e.target.value as 'en' | 'si')}>
            <option value="si">{t('lang.si')}</option>
            <option value="en">{t('lang.en')}</option>
          </select>
        )}
      </Field>
      <Field label={tl('admin.initialPassword')} hint={t('password.policy')} error={fieldError(error, 'initial_password')}>
        {(p) => <input {...p} type="password" autoComplete="new-password" value={password} onChange={(e) => setPassword(e.target.value)} />}
      </Field>
      <RoleChoices value={roles} onChange={setRoles} />
      {fieldError(error, 'roles') ? <span className="field__error">{fieldError(error, 'roles')}</span> : null}
      <div className="row">
        <button type="submit" className="btn btn--primary" disabled={busy || !email || !nameEn || roles.length === 0 || password.length < 12}>
          {t('admin.createUser')}
        </button>
        <Link className="btn" to="/admin">
          {t('common.cancel')}
        </Link>
      </div>
    </form>
  );
}

export function AdminUserScreen() {
  const { id = '' } = useParams();
  const { t, tl, name, otherName } = useT();
  const me = useMe();
  const toast = useToast();
  const u = useAsync((signal) => api.get<UserView>(`/api/v1/admin/users/${id}`, { signal }), [id]);
  const [roles, setRoles] = useState<string[] | null>(null);
  const [error, setError] = useState<unknown>(null);
  const [busy, setBusy] = useState(false);

  if (u.loading && !u.data) return <Loading />;
  if (!u.data) return <ErrorNotice error={u.error} />;
  const user = u.data;
  const shownRoles = roles ?? user.roles;

  // Each action may be answered with auth.step_up_required: the client then
  // asks for password + code and retries automatically.
  const act = async (fn: () => Promise<unknown>, done: string) => {
    setBusy(true);
    setError(null);
    try {
      await fn();
      toast('ok', done);
      clearReferenceCache();
      setRoles(null);
      u.reload();
    } catch (err) {
      setError(err);
    } finally {
      setBusy(false);
    }
  };

  return (
    <div className="stack">
      <p>
        <Link to="/admin">{t('admin.back')}</Link>
      </p>
      <h1>
        {name(user.display_name)}
        {otherName(user.display_name) ? <span className="muted">{` / ${otherName(user.display_name)}`}</span> : null}
      </h1>
      <dl className="kv">
        <dt>{tl('login.email')}</dt>
        <dd>{user.email}</dd>
        <dt>{tl('admin.status')}</dt>
        <dd>{t(`userStatus.${user.status}`)}</dd>
        <dt>{tl('admin.mfa')}</dt>
        <dd>{user.mfa_enrolled ? t('admin.mfaEnrolled') : t('admin.mfaNotEnrolled')}</dd>
        <dt>{tl('admin.lastLogin')}</dt>
        <dd>{user.last_login_at ? <Stamp at={user.last_login_at} /> : t('common.never')}</dd>
      </dl>
      <ErrorNotice error={error} />
      <section className="panel stack">
        <RoleChoices value={shownRoles} onChange={setRoles} />
        <div>
          <button
            type="button"
            className="btn btn--primary"
            disabled={busy || roles === null || shownRoles.length === 0}
            onClick={() => void act(() => api.put(`/api/v1/admin/users/${id}/roles`, { roles: shownRoles }), t('admin.rolesSaved'))}
          >
            {t('admin.saveRoles')}
          </button>
        </div>
      </section>
      <section className="panel stack">
        <h2>{tl('admin.access')}</h2>
        {user.id === me.id ? <Notice kind="info">{t('admin.selfNote')}</Notice> : null}
        <div className="row">
          {user.status === 'disabled' ? (
            <button type="button" className="btn" disabled={busy} onClick={() => void act(() => api.post(`/api/v1/admin/users/${id}/enable`), t('admin.enabled'))}>
              {t('admin.enable')}
            </button>
          ) : (
            <button
              type="button"
              className="btn btn--danger"
              disabled={busy || user.id === me.id}
              onClick={() => {
                if (window.confirm(t('admin.disableConfirm'))) void act(() => api.post(`/api/v1/admin/users/${id}/disable`), t('admin.disabled'));
              }}
            >
              {t('admin.disable')}
            </button>
          )}
          <button
            type="button"
            className="btn"
            disabled={busy}
            onClick={() => {
              if (window.confirm(t('admin.resetMfaConfirm'))) void act(() => api.post(`/api/v1/admin/users/${id}/reset-mfa`), t('admin.mfaReset'));
            }}
          >
            {t('admin.resetMfa')}
          </button>
        </div>
      </section>
    </div>
  );
}
