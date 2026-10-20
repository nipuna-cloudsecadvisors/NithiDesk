import { useEffect, useMemo, useState, type FormEvent } from 'react';
import { Link, useNavigate, useSearchParams } from 'react-router-dom';
import { api } from '../../api/client';
import { useReference, useUsers } from '../../api/reference';
import type { MatterDetail, MatterSummary } from '../../api/types';
import { useAuth } from '../../auth/AuthProvider';
import { useT } from '../../i18n/I18nProvider';
import { uuidv4 } from '../../lib/uuid';
import { Empty, ErrorNotice, Field, Loading, Notice, VerificationBadge } from '../../ui/components';
import { EntityPicker, entityPayload, type EntityChoice } from './EntityPicker';

export function MattersScreen() {
  const { t, tl, name, otherName, date } = useT();
  const { can } = useAuth();
  const [params, setParams] = useSearchParams();
  const [q, setQ] = useState(params.get('q') ?? '');
  const mine = params.get('mine') !== 'false';
  const [items, setItems] = useState<MatterSummary[]>([]);
  const [cursor, setCursor] = useState<string | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<unknown>(null);

  const load = async (append: boolean, from?: string | null) => {
    setLoading(true);
    setError(null);
    try {
      const r = await api.get<{ items: MatterSummary[] | null; next_cursor: string | null }>('/api/v1/matters', {
        query: { q: params.get('q') ?? '', mine: mine ? 'true' : undefined, cursor: from ?? undefined, limit: 50 },
      });
      setItems((prev) => (append ? [...prev, ...(r.items ?? [])] : r.items ?? []));
      setCursor(r.next_cursor);
    } catch (err) {
      setError(err);
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    void load(false);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [params]);

  const search = (e: FormEvent) => {
    e.preventDefault();
    const p = new URLSearchParams(params);
    if (q.trim()) p.set('q', q.trim());
    else p.delete('q');
    setParams(p, { replace: true });
  };

  return (
    <>
      <div className="page-head">
        <h1 className="page-head__title">{tl('nav.matters')}</h1>
        {can('matter.create') ? (
          <Link className="btn btn--primary" to="/matters/new">
            {t('matter.new')}
          </Link>
        ) : null}
      </div>
      <form className="filters" onSubmit={search} role="search">
        <Field label={tl('matter.search')}>
          {(p) => <input {...p} type="search" value={q} onChange={(e) => setQ(e.target.value)} />}
        </Field>
        <button type="submit" className="btn">
          {t('common.search')}
        </button>
        <label className="choice">
          <input
            type="checkbox"
            checked={mine}
            onChange={(e) => {
              const p = new URLSearchParams(params);
              p.set('mine', e.target.checked ? 'true' : 'false');
              setParams(p, { replace: true });
            }}
          />
          {t('matter.onlyMine')}
        </label>
      </form>
      <ErrorNotice error={error} />
      {!loading && items.length === 0 ? <Empty>{t('matter.empty')}</Empty> : null}
      <ul className="list">
        {items.map((m) => (
          <li key={m.id}>
            <Link className="list__link" to={`/matters/${m.id}`}>
              <span className="row">
                <span className="list__title">{m.matter_number}</span>
                {(m.case_numbers ?? []).map((c) => (
                  <span className="tag" key={c}>
                    {c}
                  </span>
                ))}
                {m.priority !== 'normal' ? <span className="tag tag--wait">{t(`priority.${m.priority}`)}</span> : null}
              </span>
              <span className="block">
                {m.title_masked ? t('matter.titleMasked') : name(m.title)}
                {!m.title_masked && otherName(m.title) ? <span className="muted">{` / ${otherName(m.title)}`}</span> : null}
              </span>
            </Link>
            {m.next_hearing_date ? (
              <p className="row small">
                <span>{t('matter.nextHearing', { date: date(m.next_hearing_date) })}</span>
                {m.next_hearing_verification ? <VerificationBadge state={m.next_hearing_verification} /> : null}
              </p>
            ) : null}
          </li>
        ))}
      </ul>
      {loading ? <Loading /> : null}
      {cursor && !loading ? (
        <div className="section">
          <button type="button" className="btn" onClick={() => void load(true, cursor)}>
            {t('common.loadMore')}
          </button>
        </div>
      ) : null}
    </>
  );
}

// ---------------------------------------------------------------------------

const TEAM_ROLES = ['responsible_partner', 'lead', 'supporting', 'clerk', 'assistant'] as const;
const PRIORITIES = ['normal', 'high', 'urgent'] as const;
const CONFIDENTIALITY = ['standard', 'restricted', 'highly_restricted'] as const;

export function MatterCreateScreen() {
  const { t, tl, name, fieldError, errorText } = useT();
  const navigate = useNavigate();
  const users = useUsers();
  const areas = useReference('practice-areas');
  const key = useMemo(() => uuidv4(), []);
  const [titleEn, setTitleEn] = useState('');
  const [titleSi, setTitleSi] = useState('');
  const [description, setDescription] = useState('');
  const [area, setArea] = useState('');
  const [priority, setPriority] = useState<(typeof PRIORITIES)[number]>('normal');
  const [conf, setConf] = useState<(typeof CONFIDENTIALITY)[number]>('standard');
  const [openedOn, setOpenedOn] = useState('');
  const [physical, setPhysical] = useState('');
  const [client, setClient] = useState<EntityChoice | null>(null);
  const [team, setTeam] = useState<Record<string, string>>({});
  const [error, setError] = useState<unknown>(null);
  const [busy, setBusy] = useState(false);

  const partners = Object.values(team).filter((r) => r === 'responsible_partner').length;
  const leads = Object.values(team).filter((r) => r === 'lead').length;
  const teamProblem = partners !== 1 ? t('fieldcode.matter.one_responsible_partner') : leads > 1 ? t('fieldcode.matter.one_lead') : '';

  const submit = async (e: FormEvent) => {
    e.preventDefault();
    if (teamProblem || !client) return;
    setBusy(true);
    setError(null);
    try {
      const body: Record<string, unknown> = {
        title_en: titleEn.trim(),
        title_si: titleSi.trim(),
        priority,
        confidentiality: conf,
        clients: [{ ...entityPayload(client), client_role: 'primary' }],
        team: Object.entries(team)
          .filter(([, r]) => r)
          .map(([user_id, team_role]) => ({ user_id, team_role })),
      };
      if (description.trim()) body.description = description.trim();
      if (area) body.practice_area_id = area;
      if (openedOn) body.opened_on = openedOn;
      if (physical.trim()) body.physical_file = physical.trim();
      const m = await api.post<MatterDetail>('/api/v1/matters', body, { idempotencyKey: key });
      navigate(`/matters/${m.id}`, { replace: true });
    } catch (err) {
      setError(err);
    } finally {
      setBusy(false);
    }
  };

  return (
    <form className="form form--wide" onSubmit={submit} noValidate>
      <h1>{t('matter.new')}</h1>
      {error ? <Notice kind="error">{errorText(error)}</Notice> : null}
      <div className="grid-2">
        <Field label={tl('matter.titleEn')} error={fieldError(error, 'title_en')}>
          {(p) => <input {...p} type="text" lang="en" value={titleEn} onChange={(e) => setTitleEn(e.target.value)} />}
        </Field>
        <Field label={tl('matter.titleSi')}>
          {(p) => <input {...p} type="text" lang="si" value={titleSi} onChange={(e) => setTitleSi(e.target.value)} />}
        </Field>
      </div>
      <Field label={tl('matter.description')}>
        {(p) => <textarea {...p} rows={3} value={description} onChange={(e) => setDescription(e.target.value)} />}
      </Field>
      <div className="grid-2">
        <Field label={tl('matter.practiceArea')}>
          {(p) => (
            <select {...p} value={area} onChange={(e) => setArea(e.target.value)}>
              <option value="">{t('common.choose')}</option>
              {(areas ?? []).map((a) => (
                <option key={a.id} value={a.id}>
                  {name(a.name)}
                </option>
              ))}
            </select>
          )}
        </Field>
        <Field label={tl('matter.openedOn')} error={fieldError(error, 'opened_on')}>
          {(p) => <input {...p} type="date" value={openedOn} onChange={(e) => setOpenedOn(e.target.value)} />}
        </Field>
        <Field label={tl('matter.priority')}>
          {(p) => (
            <select {...p} value={priority} onChange={(e) => setPriority(e.target.value as (typeof PRIORITIES)[number])}>
              {PRIORITIES.map((x) => (
                <option key={x} value={x}>
                  {t(`priority.${x}`)}
                </option>
              ))}
            </select>
          )}
        </Field>
        <Field label={tl('matter.confidentiality')} hint={t('matter.confidentialityHint')}>
          {(p) => (
            <select {...p} value={conf} onChange={(e) => setConf(e.target.value as (typeof CONFIDENTIALITY)[number])}>
              {CONFIDENTIALITY.map((x) => (
                <option key={x} value={x}>
                  {t(`confidentiality.${x}`)}
                </option>
              ))}
            </select>
          )}
        </Field>
      </div>
      <Field label={tl('matter.physicalFile')}>
        {(p) => <input {...p} type="text" value={physical} onChange={(e) => setPhysical(e.target.value)} />}
      </Field>

      <EntityPicker label={tl('party.client')} value={client} onChange={setClient} error={fieldError(error, 'clients')} />

      <fieldset className="field">
        <legend>{tl('matter.team')}</legend>
        <span className="field__hint">{t('matter.teamHint')}</span>
        <div className="table-wrap">
          <table className="table">
            <thead>
              <tr>
                <th scope="col">{tl('user.name')}</th>
                <th scope="col">{tl('matter.teamRole')}</th>
              </tr>
            </thead>
            <tbody>
              {(users ?? [])
                .filter((u) => u.status !== 'disabled')
                .map((u) => (
                  <tr key={u.id}>
                    <td>{name(u.display_name)}</td>
                    <td>
                      <label className="visually-hidden" htmlFor={`role-${u.id}`}>
                        {t('matter.teamRoleFor', { name: name(u.display_name) })}
                      </label>
                      <select id={`role-${u.id}`} value={team[u.id] ?? ''} onChange={(e) => setTeam({ ...team, [u.id]: e.target.value })}>
                        <option value="">{t('matter.notOnTeam')}</option>
                        {TEAM_ROLES.map((r) => (
                          <option key={r} value={r}>
                            {t(`teamRole.${r}`)}
                          </option>
                        ))}
                      </select>
                    </td>
                  </tr>
                ))}
            </tbody>
          </table>
        </div>
        {teamProblem ? <span className="field__error">{teamProblem}</span> : null}
        {fieldError(error, 'team') ? <span className="field__error">{fieldError(error, 'team')}</span> : null}
      </fieldset>

      <div className="row">
        <button type="submit" className="btn btn--primary" disabled={busy || !!teamProblem || !client || (!titleEn.trim() && !titleSi.trim())}>
          {t('matter.create')}
        </button>
        <Link className="btn" to="/matters">
          {t('common.cancel')}
        </Link>
      </div>
    </form>
  );
}
