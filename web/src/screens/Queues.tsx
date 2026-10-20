import { useMemo, useState, type FormEvent } from 'react';
import { Link } from 'react-router-dom';
import { api, type WithWarnings } from '../api/client';
import { authoritativeSources, useCourts, useReference } from '../api/reference';
import type { Card, CourtListResult, DateClaim, HearingDetail } from '../api/types';
import { isClerk, useAuth, useMe } from '../auth/AuthProvider';
import { useT } from '../i18n/I18nProvider';
import { addDays, isValidDate, todayInColombo } from '../lib/dates';
import { uuidv4 } from '../lib/uuid';
import { isExpiringSoon, queueErrorAsApiError } from '../offline/queue';
import { useQueue } from '../offline/QueueProvider';
import { Empty, ErrorNotice, Field, Loading, Notice, useAsync, useToast, Warnings } from '../ui/components';
import { HearingEntry } from '../ui/HearingEntry';
import { cleanClaim, currentAssertion, DateClaimFields, emptyClaim } from './hearing/forms';
import { useUserNames, Stamp } from './hearing/HearingScreen';

// ---------------------------------------------------------------------------
// Mobile "Queue" tab: one place for everything waiting on me.
// ---------------------------------------------------------------------------

export function QueueHub() {
  const { t, tl } = useT();
  const me = useMe();
  const { outstanding } = useQueue();
  const canVerify = me.capabilities.includes('hearing.verify') || isClerk(me);
  return (
    <>
      <h1 className="section__title">{tl('nav.queue')}</h1>
      <ul className="list">
        {outstanding > 0 ? (
          <li>
            <Link className="list__link" to="/outbox">
              <span className="list__title">{tl('nav.outbox')}</span>
              <span className="muted">{` — ${t('queue.countWaiting', { count: outstanding })}`}</span>
            </Link>
          </li>
        ) : null}
        {canVerify ? (
          <li>
            <Link className="list__link" to="/verification">
              <span className="list__title">{tl('nav.verification')}</span>
            </Link>
          </li>
        ) : null}
        {me.capabilities.includes('diary.court_list_check') ? (
          <li>
            <Link className="list__link" to="/court-list">
              <span className="list__title">{tl('nav.courtList')}</span>
            </Link>
          </li>
        ) : null}
        <li>
          <Link className="list__link" to="/outcomes-pending">
            <span className="list__title">{tl('nav.outcomesPending')}</span>
          </Link>
        </li>
      </ul>
    </>
  );
}

// ---------------------------------------------------------------------------
// Waiting to send (the offline outbox)
// ---------------------------------------------------------------------------

export function OutboxScreen() {
  const { t, tl, date, errorText } = useT();
  const { items, retry, discard, sendNow, online } = useQueue();
  const [busy, setBusy] = useState(false);
  const now = Date.now();
  return (
    <>
      <div className="page-head">
        <div className="page-head__title">
          <h1>{tl('nav.outbox')}</h1>
          <p>{t('queue.explain')}</p>
        </div>
        <button
          type="button"
          className="btn btn--primary"
          disabled={busy || !online}
          onClick={() => {
            setBusy(true);
            void sendNow().finally(() => setBusy(false));
          }}
        >
          {t('queue.sendNow')}
        </button>
      </div>
      {!online ? <Notice kind="warn">{t('offline.banner')}</Notice> : null}
      {items.length === 0 ? <Empty>{t('queue.empty')}</Empty> : null}
      <ul className="list section">
        {items.map((i) => (
          <li key={i.key} className="stack stack--tight">
            <p className="list__title">
              {t(`queue.kind.${i.kind}`)}
              {' — '}
              {i.label.caseNumber ?? t('hearing.noCaseNumber')}
              {i.label.courtDate ? ` · ${date(i.label.courtDate)}` : ''}
            </p>
            {i.label.nextDate ? <p>{t('queue.nextDate', { date: date(i.label.nextDate) })}</p> : null}
            <p className="small">
              <span className={i.status === 'needs_attention' || i.status === 'expired' ? 'tag tag--bad' : 'tag tag--wait'}>
                {t(`queue.status.${i.status}`)}
              </span>{' '}
              <span className="muted">
                {t('queue.recordedAt')} <Stamp at={new Date(i.createdAt).toISOString()} />
              </span>
            </p>
            {isExpiringSoon(i, now) ? <Notice kind="warn">{t('queue.expiresSoon')}</Notice> : null}
            {i.status === 'needs_attention' ? <Notice kind="error">{errorText(queueErrorAsApiError(i.error))}</Notice> : null}
            {i.status === 'expired' ? <Notice kind="error">{t('queue.expiredExplain')}</Notice> : null}
            <div className="row">
              <Link className="btn btn--small" to={`/hearings/${i.hearingId}`}>
                {t('hearing.open')}
              </Link>
              {i.status === 'needs_attention' ? (
                <button type="button" className="btn btn--small" onClick={() => void retry(i.key)}>
                  {t('queue.tryAgain')}
                </button>
              ) : null}
              <button
                type="button"
                className="btn btn--small btn--danger"
                onClick={() => {
                  if (window.confirm(t('queue.discardConfirm'))) void discard(i.key);
                }}
              >
                {i.status === 'expired' ? t('queue.dismiss') : t('queue.discard')}
              </button>
            </div>
          </li>
        ))}
      </ul>
    </>
  );
}

// ---------------------------------------------------------------------------
// Outcome not recorded
// ---------------------------------------------------------------------------

export function OutcomesPendingScreen() {
  const { t, tl } = useT();
  const { can } = useAuth();
  const q = useAsync((signal) => api.get<{ items: Card[] | null }>('/api/v1/queues/outcome-not-recorded', { signal }), []);
  const items = q.data?.items ?? [];
  return (
    <>
      <div className="page-head">
        <div className="page-head__title">
          <h1>{tl('nav.outcomesPending')}</h1>
          <p>{t('outcomesPending.explain')}</p>
        </div>
      </div>
      {q.loading && !q.data ? <Loading /> : null}
      <ErrorNotice error={q.error} />
      {q.data && items.length === 0 ? <Empty>{t('outcomesPending.empty')}</Empty> : null}
      <ul className="entries">
        {items.map((c) => (
          <HearingEntry
            key={c.id}
            card={c}
            showDate
            actions={
              can('hearing.outcome') ? (
                <Link className="btn btn--primary btn--small" to={`/hearings/${c.id}/outcome`}>
                  {t('hearing.recordOutcome')}
                </Link>
              ) : null
            }
          />
        ))}
      </ul>
    </>
  );
}

// ---------------------------------------------------------------------------
// Verification queue (clerks, mobile-first)
// ---------------------------------------------------------------------------

function VerifyInline({ card, onDone }: { card: Card; onDone: () => void }) {
  const { t, tl, name } = useT();
  const sources = authoritativeSources(useReference('date-source-types'));
  const [sourceId, setSourceId] = useState('');
  const [evidence, setEvidence] = useState('');
  const [error, setError] = useState<unknown>(null);
  const [busy, setBusy] = useState(false);
  const chosen = sourceId || sources.find((s) => s.code === 'official_court_list')?.id || sources[0]?.id || '';
  const save = async (e: FormEvent) => {
    e.preventDefault();
    setBusy(true);
    setError(null);
    try {
      // The current assertion id comes from the hearing detail.
      const d = await api.get<HearingDetail>(`/api/v1/hearings/${card.id}`);
      const a = currentAssertion(d);
      if (!a) throw new Error('no assertion');
      await api.post(`/api/v1/hearings/${card.id}/verifications`, {
        date_assertion_id: a.id,
        source_type_id: chosen,
        evidence_reference: evidence.trim(),
      });
      onDone();
    } catch (err) {
      setError(err);
    } finally {
      setBusy(false);
    }
  };
  return (
    <form className="form section" onSubmit={save}>
      <ErrorNotice error={error} />
      <div className="grid-2">
        <Field label={tl('verify.source')}>
          {(p) => (
            <select {...p} value={chosen} onChange={(e) => setSourceId(e.target.value)}>
              {sources.map((s) => (
                <option key={s.id} value={s.id}>
                  {name(s.name)}
                </option>
              ))}
            </select>
          )}
        </Field>
        <Field label={tl('verify.evidence')} hint={t('verify.evidenceHint')}>
          {(p) => <input {...p} type="text" value={evidence} onChange={(e) => setEvidence(e.target.value)} autoFocus />}
        </Field>
      </div>
      <div className="row">
        <button type="submit" className="btn btn--primary" disabled={busy || !evidence.trim() || !chosen}>
          {t('verify.onList')}
        </button>
      </div>
    </form>
  );
}

function DifferentDateInline({ card, onDone }: { card: Card; onDone: (w: WithWarnings<HearingDetail>['warnings']) => void }) {
  const { t } = useT();
  const sources = useReference('date-source-types');
  const [claim, setClaim] = useState<DateClaim>(() => emptyClaim(''));
  const key = useMemo(() => uuidv4(), []);
  const [error, setError] = useState<unknown>(null);
  const [busy, setBusy] = useState(false);
  const value = { ...claim, source_type_id: claim.source_type_id || sources?.find((s) => s.code === 'official_court_list')?.id || '' };
  const save = async (e: FormEvent) => {
    e.preventDefault();
    setBusy(true);
    setError(null);
    try {
      const r = await api.post<WithWarnings<HearingDetail>>(`/api/v1/hearings/${card.id}/date-assertions`, cleanClaim(value), {
        idempotencyKey: key,
      });
      onDone(r.warnings);
    } catch (err) {
      setError(err);
    } finally {
      setBusy(false);
    }
  };
  return (
    <form className="form section" onSubmit={save}>
      <p className="small">{t('verifyQueue.differentExplain')}</p>
      <ErrorNotice error={error} />
      <DateClaimFields value={value} onChange={setClaim} error={error} />
      <div className="row">
        <button type="submit" className="btn btn--primary" disabled={busy || !isValidDate(value.date) || !value.source_type_id}>
          {t('assert.submit')}
        </button>
      </div>
    </form>
  );
}

export function VerificationQueueScreen() {
  const { t, tl, name, date, weekdayName } = useT();
  const { can } = useAuth();
  const toast = useToast();
  const names = useUserNames();
  const courts = useCourts();
  const [court, setCourt] = useState('');
  const [open, setOpen] = useState<{ id: string; mode: 'verify' | 'different' } | null>(null);
  const [warnings, setWarnings] = useState<WithWarnings<unknown>['warnings']>([]);
  const q = useAsync(
    (signal) => api.get<{ items: Card[] | null }>('/api/v1/queues/verification', { query: { court, within_days: 30 }, signal }),
    [court],
  );
  const items = q.data?.items ?? [];
  const conflicts = items.filter((c) => c.verification_state === 'conflict');
  const rest = items.filter((c) => c.verification_state !== 'conflict');
  const byDate = new Map<string, Card[]>();
  for (const c of rest) byDate.set(c.date, [...(byDate.get(c.date) ?? []), c]);
  const canVerify = can('hearing.verify');

  const actions = (c: Card) =>
    canVerify ? (
      <>
        <button type="button" className="btn btn--primary btn--small" aria-expanded={open?.id === c.id && open.mode === 'verify'} onClick={() => setOpen({ id: c.id, mode: 'verify' })}>
          {t('verify.action')}
        </button>
        <button type="button" className="btn btn--small" aria-expanded={open?.id === c.id && open.mode === 'different'} onClick={() => setOpen({ id: c.id, mode: 'different' })}>
          {t('verifyQueue.differentDate')}
        </button>
        {open?.id === c.id ? (
          <div className="grow">
            {open.mode === 'verify' ? (
              <VerifyInline
                card={c}
                onDone={() => {
                  setOpen(null);
                  toast('ok', t('verify.done'));
                  q.reload();
                }}
              />
            ) : (
              <DifferentDateInline
                card={c}
                onDone={(w) => {
                  setOpen(null);
                  setWarnings(w);
                  toast('ok', t('assert.done'));
                  q.reload();
                }}
              />
            )}
          </div>
        ) : null}
      </>
    ) : null;

  return (
    <>
      <div className="page-head">
        <div className="page-head__title">
          <h1>{tl('nav.verification')}</h1>
          <p>{t('verifyQueue.explain')}</p>
        </div>
        {can('diary.court_list_check') ? (
          <Link className="btn" to="/court-list">
            {t('verifyQueue.checkList')}
          </Link>
        ) : null}
      </div>
      <div className="filters">
        <Field label={tl('court.label')}>
          {(p) => (
            <select {...p} value={court} onChange={(e) => setCourt(e.target.value)}>
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
      <Warnings warnings={warnings} names={names} />
      {q.loading && !q.data ? <Loading /> : null}
      <ErrorNotice error={q.error} />
      {q.data && items.length === 0 ? <Empty>{t('verifyQueue.empty')}</Empty> : null}
      {conflicts.length > 0 ? (
        <section className="court-group">
          <div className="court-group__head">
            <h2>
              <span aria-hidden="true">{'⚠ '}</span>
              {tl('verifyQueue.conflicts')}
            </h2>
            <span className="court-group__count">{t('today.hearingCount', { count: conflicts.length })}</span>
          </div>
          <ul className="entries">
            {conflicts.map((c) => (
              <HearingEntry key={c.id} card={c} showDate actions={actions(c)} />
            ))}
          </ul>
        </section>
      ) : null}
      {[...byDate.entries()].map(([d, cards]) => (
        <section className="court-group" key={d}>
          <div className="court-group__head">
            <h2>
              {weekdayName(d)} {date(d)}
            </h2>
            <span className="court-group__count">{t('today.hearingCount', { count: cards.length })}</span>
          </div>
          <ul className="entries">
            {cards.map((c) => (
              <HearingEntry key={c.id} card={c} actions={actions(c)} />
            ))}
          </ul>
        </section>
      ))}
    </>
  );
}

// ---------------------------------------------------------------------------
// Court-list check
// ---------------------------------------------------------------------------

export function CourtListCheckScreen() {
  const { t, tl, name, longDate } = useT();
  const courts = useCourts();
  const [court, setCourt] = useState('');
  const [listDate, setListDate] = useState(() => addDays(todayInColombo(), 1));
  const [numbers, setNumbers] = useState('');
  const [verify, setVerify] = useState(false);
  const [detail, setDetail] = useState('');
  const [result, setResult] = useState<CourtListResult | null>(null);
  const [error, setError] = useState<unknown>(null);
  const [busy, setBusy] = useState(false);

  const lines = numbers
    .split(/\r?\n/)
    .map((s) => s.trim())
    .filter(Boolean);

  const run = async (e: FormEvent) => {
    e.preventDefault();
    setBusy(true);
    setError(null);
    setResult(null);
    try {
      const body: Record<string, unknown> = { court_id: court, list_date: listDate, numbers: lines, verify_matches: verify };
      if (detail.trim()) body.source_detail = detail.trim();
      setResult(await api.post<CourtListResult>('/api/v1/diary/court-list-check', body));
    } catch (err) {
      setError(err);
    } finally {
      setBusy(false);
    }
  };

  const matched = result?.matched ?? [];
  const different = result?.different_date ?? [];
  const missing = result?.missing ?? [];
  const unexpected = result?.unexpected ?? [];

  return (
    <>
      <div className="page-head">
        <div className="page-head__title">
          <h1>{tl('nav.courtList')}</h1>
          <p>{t('courtList.explain')}</p>
        </div>
      </div>
      <form className="form" onSubmit={run}>
        <ErrorNotice error={error} />
        <div className="grid-2">
          <Field label={tl('court.label')}>
            {(p) => (
              <select {...p} required value={court} onChange={(e) => setCourt(e.target.value)}>
                <option value="">{t('common.choose')}</option>
                {(courts ?? []).map((c) => (
                  <option key={c.id} value={c.id}>
                    {name(c.name)}
                  </option>
                ))}
              </select>
            )}
          </Field>
          <Field label={tl('courtList.date')} hint={isValidDate(listDate) ? longDate(listDate) : undefined}>
            {(p) => <input {...p} type="date" required value={listDate} onChange={(e) => setListDate(e.target.value)} />}
          </Field>
        </div>
        <Field label={tl('courtList.numbers')} hint={t('courtList.numbersHint', { count: lines.length })}>
          {(p) => <textarea {...p} rows={8} value={numbers} onChange={(e) => setNumbers(e.target.value)} spellCheck={false} />}
        </Field>
        <Field label={tl('courtList.sourceDetail')} hint={t('courtList.sourceDetailHint')}>
          {(p) => <input {...p} type="text" value={detail} onChange={(e) => setDetail(e.target.value)} />}
        </Field>
        <label className="choice">
          <input type="checkbox" checked={verify} onChange={(e) => setVerify(e.target.checked)} />
          {t('courtList.verifyMatches')}
        </label>
        <button type="submit" className="btn btn--primary" disabled={busy || !court || !isValidDate(listDate)}>
          {t('courtList.run')}
        </button>
      </form>

      {result ? (
        <div className="section stack">
          <ul className="badges" aria-label={t('courtList.summary')}>
            <li className="badge badge--ok">
              <span aria-hidden="true">✓</span>
              {t('courtList.matchedCount', { count: matched.length })}
            </li>
            <li className={different.length ? 'badge badge--bad' : 'badge'}>
              <span aria-hidden="true">⚠</span>
              {t('courtList.differentCount', { count: different.length })}
            </li>
            <li className={missing.length ? 'badge badge--bad' : 'badge'}>
              <span aria-hidden="true">✗</span>
              {t('courtList.missingCount', { count: missing.length })}
            </li>
            <li className={unexpected.length ? 'badge badge--bad' : 'badge'}>
              <span aria-hidden="true">!</span>
              {t('courtList.unexpectedCount', { count: unexpected.length })}
            </li>
          </ul>

          {different.length > 0 ? (
            <section>
              <h2 className="section__title">{tl('courtList.different')}</h2>
              <Notice kind="error">{t('courtList.differentExplain')}</Notice>
              <ul className="list section">
                {different.map((m) => (
                  <li key={m.hearing_id}>
                    <Link to={`/hearings/${m.hearing_id}`}>{m.number}</Link>
                  </li>
                ))}
              </ul>
            </section>
          ) : null}
          {missing.length > 0 ? (
            <section>
              <h2 className="section__title">{tl('courtList.missing')}</h2>
              <Notice kind="error">{t('courtList.missingExplain')}</Notice>
              <ul className="entries section">
                {missing.map((c) => (
                  <HearingEntry key={c.id} card={c} />
                ))}
              </ul>
            </section>
          ) : null}
          {unexpected.length > 0 ? (
            <section>
              <h2 className="section__title">{tl('courtList.unexpected')}</h2>
              <Notice kind="error">{t('courtList.unexpectedExplain')}</Notice>
              <ul className="list section">
                {unexpected.map((n) => (
                  <li key={n}>
                    <strong>{n}</strong>
                  </li>
                ))}
              </ul>
            </section>
          ) : null}
          {matched.length > 0 ? (
            <section>
              <h2 className="section__title">{tl('courtList.matched')}</h2>
              <ul className="list">
                {matched.map((m) => (
                  <li key={m.hearing_id} className="row">
                    <Link to={`/hearings/${m.hearing_id}`}>{m.number}</Link>
                    {m.verified ? <span className="tag tag--ok">{`✓ ${t('verification.verified')}`}</span> : null}
                  </li>
                ))}
              </ul>
            </section>
          ) : null}
        </div>
      ) : null}
    </>
  );
}
