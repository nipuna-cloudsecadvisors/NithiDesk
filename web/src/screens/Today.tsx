import { Link, useSearchParams } from 'react-router-dom';
import { api } from '../api/client';
import type { DayView } from '../api/types';
import { useAuth } from '../auth/AuthProvider';
import { useT } from '../i18n/I18nProvider';
import { addDays, compareDates, isValidDate, longParts, todayInColombo } from '../lib/dates';
import { useQueue } from '../offline/QueueProvider';
import { Empty, ErrorNotice, Loading, Notice, Segmented, useAsync } from '../ui/components';
import { CourtGroups } from '../ui/HearingEntry';

type Scope = 'mine' | 'team' | 'firm';

/** The diary page header: the day numeral, weekday and month. */
export function DiaryHead({ date, onPrev, onNext, onToday }: { date: string; onPrev: () => void; onNext: () => void; onToday?: () => void }) {
  const { t, longDate } = useT();
  const p = longParts(date);
  if (!p) return null;
  const today = todayInColombo();
  return (
    <header className="diary-head">
      <span className="diary-head__day" aria-hidden="true">
        {p.day}
      </span>
      <span className="diary-head__rest">
        <h1 className="diary-head__weekday">
          {date === today ? t('today.titleToday', { weekday: t(`date.weekday.${p.weekday}`) }) : t(`date.weekday.${p.weekday}`)}
          <span className="visually-hidden">{t('today.fullDate', { date: longDate(date) })}</span>
        </h1>
        <span className="diary-head__month">{t('date.monthYear', { month: t(`date.month.${p.month}`), year: String(p.year) })}</span>
      </span>
      <span className="diary-head__nav no-print">
        <button type="button" className="btn btn--small" onClick={onPrev} aria-label={t('today.prevDay')}>
          {t('common.prevArrow')}
        </button>
        {onToday && date !== today ? (
          <button type="button" className="btn btn--small" onClick={onToday}>
            {t('today.backToToday')}
          </button>
        ) : null}
        <button type="button" className="btn btn--small" onClick={onNext} aria-label={t('today.nextDay')}>
          {t('common.nextArrow')}
        </button>
      </span>
    </header>
  );
}

export function TodayScreen() {
  const { t, tl } = useT();
  const { can } = useAuth();
  const { outstanding } = useQueue();
  const [params, setParams] = useSearchParams();
  const today = todayInColombo();
  const raw = params.get('date') ?? '';
  const date = isValidDate(raw) ? raw : today;
  const scope = (['mine', 'team', 'firm'].includes(params.get('scope') ?? '') ? params.get('scope') : 'mine') as Scope;
  const set = (next: { date?: string; scope?: Scope }) => {
    const p = new URLSearchParams(params);
    if (next.date !== undefined) {
      if (next.date === today) p.delete('date');
      else p.set('date', next.date);
    }
    if (next.scope) p.set('scope', next.scope);
    setParams(p, { replace: true });
  };

  const view = useAsync(
    (signal) => api.get<DayView>('/api/v1/diary/today', { query: { scope, date }, signal }),
    [scope, date],
  );
  const d = view.data;
  const counts = d?.counts;
  const canOutcome = can('hearing.outcome');

  return (
    <>
      <DiaryHead
        date={date}
        onPrev={() => set({ date: addDays(date, -1) })}
        onNext={() => set({ date: addDays(date, 1) })}
        onToday={() => set({ date: today })}
      />
      <div className="row">
        <Segmented<Scope>
          label={t('scope.label')}
          value={scope}
          onChange={(s) => set({ scope: s })}
          options={[
            { value: 'mine', label: tl('scope.mine') },
            { value: 'team', label: tl('scope.team') },
            { value: 'firm', label: tl('scope.firm') },
          ]}
        />
        <Link className="btn btn--quiet btn--small" to={`/brief?date=${date}&scope=${scope}`}>
          {t('today.openBrief')}
        </Link>
      </div>

      {counts ? (
        <ul className="badges section" aria-label={t('today.badgesLabel')}>
          {counts.conflicts > 0 ? (
            <li>
              <Link className="badge badge--bad" to="/verification">
                <span aria-hidden="true">⚠</span>
                {t('today.badge.conflicts', { count: counts.conflicts })}
              </Link>
            </li>
          ) : null}
          {counts.unverified > 0 ? (
            <li>
              <Link className="badge badge--wait" to="/verification">
                <span aria-hidden="true">?</span>
                {t('today.badge.unverified', { count: counts.unverified })}
              </Link>
            </li>
          ) : null}
          {counts.outcomes_not_recorded > 0 ? (
            <li>
              <Link className="badge badge--wait" to="/outcomes-pending">
                <span aria-hidden="true">✎</span>
                {t('today.badge.notRecorded', { count: counts.outcomes_not_recorded })}
              </Link>
            </li>
          ) : null}
          {counts.clashes > 0 ? (
            <li>
              <Link className="badge badge--bad" to={`/week?start=${date}`}>
                <span aria-hidden="true">⇄</span>
                {t('today.badge.clashes', { count: counts.clashes })}
              </Link>
            </li>
          ) : null}
          {outstanding > 0 ? (
            <li>
              <Link className="badge badge--wait" to="/outbox">
                <span aria-hidden="true">↑</span>
                {t('today.badge.waitingToSend', { count: outstanding })}
              </Link>
            </li>
          ) : null}
          {counts.conflicts + counts.unverified + counts.outcomes_not_recorded + counts.clashes === 0 ? (
            <li>
              <span className="badge badge--ok">
                <span aria-hidden="true">✓</span>
                {t('today.badge.allClear')}
              </span>
            </li>
          ) : null}
        </ul>
      ) : null}

      {(d?.calendar_days ?? []).map((c) => (
        <Notice key={c.id} kind="warn">
          {t(`calendar.kind.${c.kind}`)}
          {c.label_en || c.label_si ? `: ${t('common.bilingual', { en: c.label_en ?? '', si: c.label_si ?? '' })}` : ''}
        </Notice>
      ))}

      {view.loading && !d ? <Loading /> : null}
      <ErrorNotice error={view.error} />
      {d && (d.courts ?? []).length === 0 ? (
        <Empty>
          <p>{scope === 'mine' ? t('today.emptyMine') : t('today.empty')}</p>
          {scope === 'mine' ? (
            <button type="button" className="btn btn--quiet" onClick={() => set({ scope: 'team' })}>
              {t('today.showTeam')}
            </button>
          ) : null}
        </Empty>
      ) : null}
      {d ? (
        <CourtGroups
          courts={d.courts}
          actions={(c) =>
            canOutcome && compareDates(c.date, today) <= 0 && c.listing_status !== 'outcome_recorded' && c.listing_status !== 'vacated' && c.listing_status !== 'cancelled' ? (
              <Link className="btn btn--primary btn--small" to={`/hearings/${c.id}/outcome`}>
                {t('hearing.recordOutcome')}
              </Link>
            ) : (
              <Link className="btn btn--small" to={`/hearings/${c.id}`}>
                {t('hearing.open')}
              </Link>
            )
          }
        />
      ) : null}
    </>
  );
}
