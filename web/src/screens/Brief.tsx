import { useEffect, useRef, useState } from 'react';
import { useSearchParams } from 'react-router-dom';
import { api } from '../api/client';
import type { BriefRender, DayView } from '../api/types';
import { useAuth } from '../auth/AuthProvider';
import { useT } from '../i18n/I18nProvider';
import { addDays, isValidDate, todayInColombo } from '../lib/dates';
import { Empty, ErrorNotice, Field, Loading, Notice, Segmented, useAsync } from '../ui/components';
import { CourtGroups } from '../ui/HearingEntry';
import { DiaryHead } from './Today';

type Scope = 'mine' | 'team' | 'firm';
type Lang = 'en' | 'si' | 'both';

/** The brief API may answer one day (a DayView) or several ({days:[…]}). */
function daysOf(raw: unknown): DayView[] {
  if (!raw || typeof raw !== 'object') return [];
  const r = raw as { courts?: unknown; days?: unknown };
  if (Array.isArray(r.days)) return r.days as DayView[];
  if (Array.isArray(r.courts) || r.courts === null) return [raw as DayView];
  return [];
}

const POLL_MS = 2000;
const POLL_LIMIT = 60;

export function BriefScreen() {
  const { t, tl, longDate } = useT();
  const { can } = useAuth();
  const [params, setParams] = useSearchParams();
  const date = isValidDate(params.get('date') ?? '') ? (params.get('date') as string) : todayInColombo();
  const scope = (['mine', 'team', 'firm'].includes(params.get('scope') ?? '') ? params.get('scope') : 'mine') as Scope;
  const [days, setDays] = useState(1);
  const [lang, setLang] = useState<Lang>('both');
  const [render, setRender] = useState<BriefRender | null>(null);
  const [renderError, setRenderError] = useState<unknown>(null);
  const timer = useRef<number | null>(null);

  const set = (k: string, v: string) => {
    const p = new URLSearchParams(params);
    p.set(k, v);
    setParams(p, { replace: true });
  };
  const brief = useAsync(
    (signal) => api.get<unknown>('/api/v1/briefs/daily', { query: { date, days, scope }, signal }),
    [date, days, scope],
  );
  const views = daysOf(brief.data);

  useEffect(() => () => {
    if (timer.current) window.clearTimeout(timer.current);
  }, []);

  const poll = (id: string, n: number) => {
    timer.current = window.setTimeout(async () => {
      try {
        const r = await api.get<BriefRender>(`/api/v1/briefs/renders/${id}`);
        setRender(r);
        if (r.status !== 'ready' && r.status !== 'failed' && n < POLL_LIMIT) poll(id, n + 1);
      } catch (err) {
        setRenderError(err);
      }
    }, POLL_MS);
  };

  const print = async () => {
    setRenderError(null);
    setRender(null);
    try {
      const r = await api.post<BriefRender>('/api/v1/briefs/daily/renders', { date, days, scope, lang });
      setRender(r);
      if (r.status !== 'ready' && r.status !== 'failed') poll(r.id, 0);
    } catch (err) {
      setRenderError(err);
    }
  };

  const scopes: { value: Scope; label: string }[] = [
    { value: 'mine', label: tl('scope.mine') },
    { value: 'team', label: tl('scope.team') },
  ];
  if (can('brief.print_firm')) scopes.push({ value: 'firm', label: tl('scope.firm') });

  return (
    <>
      <DiaryHead date={date} onPrev={() => set('date', addDays(date, -1))} onNext={() => set('date', addDays(date, 1))} />
      <div className="filters no-print">
        <Segmented<Scope> label={t('scope.label')} value={scope} onChange={(s) => set('scope', s)} options={scopes} />
        <Field label={tl('brief.days')}>
          {(p) => (
            <select {...p} value={days} onChange={(e) => setDays(Number(e.target.value))}>
              {[1, 2, 3, 5, 7].map((n) => (
                <option key={n} value={n}>
                  {t('brief.dayCount', { count: n })}
                </option>
              ))}
            </select>
          )}
        </Field>
        <Field label={tl('brief.lang')}>
          {(p) => (
            <select {...p} value={lang} onChange={(e) => setLang(e.target.value as Lang)}>
              <option value="en">{t('lang.en')}</option>
              <option value="si">{t('lang.si')}</option>
              <option value="both">{t('lang.both')}</option>
            </select>
          )}
        </Field>
        <button type="button" className="btn btn--primary" onClick={() => void print()} disabled={render?.status === 'queued' || render?.status === 'rendering'}>
          {t('brief.printPdf')}
        </button>
      </div>
      <div className="no-print stack">
        <ErrorNotice error={renderError} />
        {render && (render.status === 'queued' || render.status === 'rendering') ? <Notice kind="info">{t('brief.rendering')}</Notice> : null}
        {render?.status === 'failed' ? <Notice kind="error">{t('brief.failed')}</Notice> : null}
        {render?.status === 'ready' ? (
          <Notice kind="ok">
            {t('brief.ready')}{' '}
            <a href={`/api/v1/briefs/renders/${render.id}/file`} download>
              {t('brief.download')}
            </a>
          </Notice>
        ) : null}
      </div>

      {brief.loading && !brief.data ? <Loading /> : null}
      <ErrorNotice error={brief.error} />
      {brief.data && views.every((v) => (v.courts ?? []).length === 0) ? <Empty>{t('brief.empty')}</Empty> : null}
      {views.map((v) => (
        <section key={v.date} className="section">
          {views.length > 1 ? <h2 className="section__title">{longDate(v.date)}</h2> : null}
          <CourtGroups courts={v.courts} />
        </section>
      ))}
    </>
  );
}
