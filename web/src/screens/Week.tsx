import { Link, useSearchParams } from 'react-router-dom';
import { api } from '../api/client';
import { useCourts, useUsers } from '../api/reference';
import type { WeekView } from '../api/types';
import { useT } from '../i18n/I18nProvider';
import { addDays, isValidDate, todayInColombo } from '../lib/dates';
import { Empty, ErrorNotice, Field, Loading, Segmented, useAsync, VerificationBadge } from '../ui/components';
import { sessionLabel, usePartiesLine } from '../ui/HearingEntry';

type Scope = 'mine' | 'team' | 'firm';

export function WeekScreen() {
  const { t, tl, name, date, weekdayName } = useT();
  const parties = usePartiesLine();
  const courts = useCourts();
  const users = useUsers();
  const [params, setParams] = useSearchParams();
  const today = todayInColombo();
  const start = isValidDate(params.get('start') ?? '') ? (params.get('start') as string) : today;
  const scope = (['mine', 'team', 'firm'].includes(params.get('scope') ?? '') ? params.get('scope') : 'team') as Scope;
  const lawyer = params.get('lawyer') ?? '';
  const court = params.get('court') ?? '';
  const set = (k: string, v: string) => {
    const p = new URLSearchParams(params);
    if (v) p.set(k, v);
    else p.delete(k);
    setParams(p, { replace: true });
  };
  const w = useAsync(
    (signal) => api.get<WeekView>('/api/v1/diary/week', { query: { start, days: 7, scope, lawyer, court }, signal }),
    [start, scope, lawyer, court],
  );
  const v = w.data;
  const hl = v?.highlights;
  const lawyers = (users ?? []).filter((u) => u.status !== 'disabled');

  const items: { key: keyof NonNullable<typeof hl>; label: string; hot: boolean }[] = hl
    ? [
        { key: 'conflicts', label: t('week.hl.conflicts'), hot: true },
        { key: 'verification_overdue', label: t('week.hl.overdue'), hot: true },
        { key: 'clashes', label: t('week.hl.clashes'), hot: true },
        { key: 'not_ready', label: t('week.hl.notReady'), hot: false },
        { key: 'no_appearing_lawyer', label: t('week.hl.noLawyer'), hot: false },
      ]
    : [];

  return (
    <>
      <div className="page-head">
        <div className="page-head__title">
          <h1>{tl('nav.diary')}</h1>
          {v ? <p>{t('week.range', { from: date(v.start), to: date(v.end) })}</p> : null}
        </div>
        <div className="row">
          <button type="button" className="btn btn--small" onClick={() => set('start', addDays(start, -7))} aria-label={t('week.prev')}>
            {t('common.prevArrow')}
          </button>
          <button type="button" className="btn btn--small" onClick={() => set('start', '')}>
            {t('week.thisWeek')}
          </button>
          <button type="button" className="btn btn--small" onClick={() => set('start', addDays(start, 7))} aria-label={t('week.next')}>
            {t('common.nextArrow')}
          </button>
        </div>
      </div>

      <div className="filters">
        <Segmented<Scope>
          label={t('scope.label')}
          value={scope}
          onChange={(s) => set('scope', s)}
          options={[
            { value: 'mine', label: tl('scope.mine') },
            { value: 'team', label: tl('scope.team') },
            { value: 'firm', label: tl('scope.firm') },
          ]}
        />
        <Field label={tl('week.lawyer')}>
          {(p) => (
            <select {...p} value={lawyer} onChange={(e) => set('lawyer', e.target.value)}>
              <option value="">{t('week.allLawyers')}</option>
              {lawyers.map((u) => (
                <option key={u.id} value={u.id}>
                  {name(u.display_name)}
                </option>
              ))}
            </select>
          )}
        </Field>
        <Field label={tl('court.label')}>
          {(p) => (
            <select {...p} value={court} onChange={(e) => set('court', e.target.value)}>
              <option value="">{t('common.allCourts')}</option>
              {(courts ?? []).map((c) => (
                <option key={c.id} value={c.id}>
                  {name(c.name)}
                </option>
              ))}
            </select>
          )}
        </Field>
      </div>

      {hl ? (
        <div className="highlights" role="list" aria-label={t('week.highlights')}>
          {items.map((i) => {
            const n = hl[i.key];
            const cls = n > 0 ? (i.hot ? 'highlight highlight--hot' : 'highlight highlight--warm') : 'highlight';
            return (
              <div className={cls} role="listitem" key={i.key}>
                <strong>{n}</strong>
                {i.label}
              </div>
            );
          })}
        </div>
      ) : null}

      {w.loading && !v ? <Loading /> : null}
      <ErrorNotice error={w.error} />
      {v && (v.days ?? []).length === 0 ? <Empty>{t('week.empty')}</Empty> : null}
      {(v?.days ?? []).map((day) => (
        <section className="week-day" key={day.date} aria-labelledby={`d-${day.date}`}>
          <div className={day.date === today ? 'week-day__head is-today' : 'week-day__head'}>
            <h2 id={`d-${day.date}`}>
              {weekdayName(day.date)} {date(day.date)}
            </h2>
            <span className="muted small">{t('today.hearingCount', { count: (day.hearings ?? []).length })}</span>
          </div>
          <ul className="week-rows">
            {(day.hearings ?? []).map((c) => (
              <li className="week-row" key={c.id}>
                <span>
                  <Link to={`/hearings/${c.id}`}>{c.case_number ?? t('hearing.noCaseNumber')}</Link>
                  <br />
                  <VerificationBadge state={c.verification_state} overdue={c.verification_overdue} />
                </span>
                <span>
                  {name(c.court.name)}
                  {sessionLabel(t, c) ? <span className="muted">{` · ${sessionLabel(t, c)}`}</span> : null}
                </span>
                <span>
                  {c.title_masked ? t('matter.titleMasked') : parties(c.our_clients, c.opposing) || name(c.matter_title)}
                  {c.purpose ? <span className="muted">{` · ${name(c.purpose.name)}`}</span> : null}
                </span>
                <span>
                  {(c.appearing ?? []).map((a) => a.name).join(', ') || <span className="tag tag--wait">{t('hearing.noAppearingLawyer')}</span>}
                  {c.clash ? <span className="tag tag--bad">{` ${t('hearing.clash')}`}</span> : null}
                </span>
                <span>
                  <span className={c.preparation_status === 'ready' ? 'tag tag--ok' : c.preparation_status === 'blocked' ? 'tag tag--bad' : 'tag tag--wait'}>
                    {tl(`preparation.${c.preparation_status}`)}
                  </span>
                </span>
              </li>
            ))}
          </ul>
        </section>
      ))}
    </>
  );
}
