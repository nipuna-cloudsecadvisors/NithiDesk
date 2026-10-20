import { useEffect, useMemo, useState, type FormEvent } from 'react';
import { Link, useNavigate, useParams } from 'react-router-dom';
import { api } from '../../api/client';
import { getCalendar, useReference } from '../../api/reference';
import type { CalendarDay, HearingDetail, OutcomeInput, RefItem, Session } from '../../api/types';
import { useMe } from '../../auth/AuthProvider';
import { useT } from '../../i18n/I18nProvider';
import { compareDates, isValidDate, isWeekend, todayInColombo } from '../../lib/dates';
import { uuidv4 } from '../../lib/uuid';
import { queueErrorAsApiError, type QueueItem } from '../../offline/queue';
import { useQueue } from '../../offline/QueueProvider';
import { ErrorNotice, Field, Loading, Notice, useAsync, useToast } from '../../ui/components';
import { SESSIONS } from './forms';

const NO_NEXT_REASONS = ['concluded', 'laid_by', 'date_from_registry', 'other'] as const;
type NoNextReason = (typeof NO_NEXT_REASONS)[number];

interface FormState {
  outcomeTypeId: string;
  hasNext: boolean;
  nextDate: string;
  session: Session;
  time: string;
  purposeId: string;
  purposeNote: string;
  noNextReason: NoNextReason | '';
  noNextNote: string;
  orderText: string;
  informClient: boolean;
  certifiedCopy: boolean;
  appearedUserId: string;
}

const initial: FormState = {
  outcomeTypeId: '',
  hasNext: true,
  nextDate: '',
  session: 'unspecified',
  time: '',
  purposeId: '',
  purposeNote: '',
  noNextReason: '',
  noNextNote: '',
  orderText: '',
  informClient: true,
  certifiedCopy: false,
  appearedUserId: '',
};

/** Default "no next date" reason for an outcome type. */
function defaultReason(type: RefItem): NoNextReason {
  if (type.code === 'laid_by') return 'laid_by';
  if (type.attributes?.concludes === true) return 'concluded';
  return 'date_from_registry';
}

export interface ReadbackWarning {
  key: string;
  values?: Record<string, string>;
}

/** Warnings shown on the read-back step for a next date (doc 12 §3.4). */
export function nextDateWarnings(next: string, hearingDate: string, today: string, calendar: CalendarDay[], courtId: string): ReadbackWarning[] {
  const out: ReadbackWarning[] = [];
  if (compareDates(next, today) < 0) out.push({ key: 'outcome.warn.past' });
  if (isWeekend(next)) out.push({ key: 'outcome.warn.weekend' });
  for (const c of calendar) {
    if (c.day === next && (c.court_id === null || c.court_id === courtId)) {
      out.push({ key: `outcome.warn.calendar.${c.kind}`, values: { label: c.label_en ?? c.label_si ?? '' } });
    }
  }
  if (compareDates(next, hearingDate) <= 0) out.push({ key: 'outcome.warn.notAfter' });
  return out;
}

export function RecordOutcomeScreen() {
  const { id = '' } = useParams();
  const { t, tl, name, longDate, date, errorText } = useT();
  const me = useMe();
  const navigate = useNavigate();
  const toast = useToast();
  const { submit } = useQueue();
  const types = useReference('outcome-types');
  const purposes = useReference('hearing-purposes');
  const h = useAsync((signal) => api.get<HearingDetail>(`/api/v1/hearings/${id}`, { signal }), [id]);
  const [f, setF] = useState<FormState>(initial);
  const [step, setStep] = useState<'form' | 'confirm'>('form');
  const [calendar, setCalendar] = useState<CalendarDay[]>([]);
  const [busy, setBusy] = useState(false);
  const [problem, setProblem] = useState<QueueItem | null>(null);
  const [formError, setFormError] = useState('');
  // One key and one next-hearing id per form: a resend is a replay, never a duplicate.
  const idemKey = useMemo(() => uuidv4(), []);
  const nextId = useMemo(() => uuidv4(), []);

  const set = <K extends keyof FormState>(k: K, v: FormState[K]) => setF((s) => ({ ...s, [k]: v }));
  const type = types?.find((x) => x.id === f.outcomeTypeId);

  useEffect(() => {
    if (step !== 'confirm' || !f.hasNext || !isValidDate(f.nextDate)) return;
    getCalendar(f.nextDate, f.nextDate)
      .then(setCalendar)
      .catch(() => setCalendar([]));
  }, [step, f.hasNext, f.nextDate]);

  if (h.loading && !h.data) return <Loading />;
  if (!h.data) return <ErrorNotice error={h.error} />;
  const d = h.data;
  const today = todayInColombo();
  const caseNo = d.case_number ?? t('hearing.noCaseNumber');

  const chooseType = (item: RefItem) => {
    const requiresNext = item.attributes?.requires_next === true;
    setF((s) => ({
      ...s,
      outcomeTypeId: item.id,
      hasNext: requiresNext,
      noNextReason: requiresNext ? s.noNextReason : defaultReason(item),
    }));
  };

  const toConfirm = (e: FormEvent) => {
    e.preventDefault();
    setFormError('');
    if (!type) return setFormError(t('outcome.err.type'));
    if (f.hasNext) {
      if (!isValidDate(f.nextDate)) return setFormError(t('outcome.err.nextDate'));
      if (compareDates(f.nextDate, d.date) <= 0) return setFormError(t('outcome.warn.notAfter'));
    } else if (!f.noNextReason) {
      return setFormError(t('outcome.err.reason'));
    }
    setStep('confirm');
    window.scrollTo(0, 0);
  };

  const body = (): OutcomeInput => {
    const out: OutcomeInput = {
      outcome_type_id: f.outcomeTypeId,
      inform_client: f.informClient,
      certified_copy: f.certifiedCopy,
      client_recorded_at: new Date().toISOString(),
    };
    if (f.orderText.trim()) out.order_text = f.orderText.trim();
    if (f.appearedUserId) out.appeared_user_id = f.appearedUserId;
    if (f.hasNext) {
      out.next_hearing = { id: nextId, date: f.nextDate, session: f.session };
      if (f.time) out.next_hearing.time = f.time;
      if (f.purposeId) out.next_hearing.purpose_id = f.purposeId;
      if (f.purposeNote.trim()) out.next_hearing.purpose_note = f.purposeNote.trim();
    } else {
      out.no_next_reason = f.noNextReason;
      if (f.noNextNote.trim()) out.no_next_note = f.noNextNote.trim();
    }
    return out;
  };

  const save = async () => {
    setBusy(true);
    setProblem(null);
    try {
      const left = await submit({
        key: idemKey,
        kind: 'outcome',
        hearingId: d.id,
        method: 'PUT',
        path: `/api/v1/hearings/${d.id}/outcome`,
        body: body(),
        label: { caseNumber: d.case_number, courtDate: d.date, nextDate: f.hasNext ? f.nextDate : undefined },
      });
      if (!left) {
        toast('ok', f.hasNext ? t('outcome.savedWithNext', { date: longDate(f.nextDate) }) : t('outcome.saved'));
        navigate(`/hearings/${d.id}`, { replace: true });
      } else if (left.status === 'needs_attention') {
        setProblem(left);
      } else {
        toast('warn', t('queue.savedWaiting'));
        navigate('/today', { replace: true });
      }
    } finally {
      setBusy(false);
    }
  };

  const purpose = purposes?.find((p) => p.id === f.purposeId);
  const warns = f.hasNext && isValidDate(f.nextDate) ? nextDateWarnings(f.nextDate, d.date, today, calendar, d.court.id) : [];
  const appearing = d.appearing ?? [];

  if (step === 'confirm') {
    return (
      <div className="stack">
        <h1>{t('outcome.confirmTitle', { caseNo })}</h1>
        <div className="readback" aria-live="polite">
          <p className="muted">{tl(`outcome.typeLabel`)}</p>
          <p className="readback__purpose">{type ? name(type.name) : ''}</p>
          {f.hasNext ? (
            <>
              <p className="muted section">{tl('outcome.nextDate')}</p>
              <p className="readback__date">{longDate(f.nextDate)}</p>
              <p className="readback__purpose">
                {[purpose ? name(purpose.name) : '', f.purposeNote, f.session !== 'unspecified' ? t(`session.${f.session}`) : '', f.time]
                  .filter(Boolean)
                  .join(' — ')}
              </p>
              <p className="section">
                <strong>{t('outcome.isCorrect')}</strong>
              </p>
            </>
          ) : (
            <>
              <p className="muted section">{tl('outcome.noNextDate')}</p>
              <p className="readback__purpose">{t(`outcome.noNext.${f.noNextReason}`)}</p>
            </>
          )}
        </div>
        {warns.map((w) => (
          <Notice key={w.key} kind="warn">
            {t(w.key, w.values)}
          </Notice>
        ))}
        {problem ? (
          <Notice kind="error">
            {errorText(queueErrorAsApiError(problem.error))}{' '}
            <Link to="/outbox">{t('queue.open')}</Link>
          </Notice>
        ) : null}
        <p className="small muted">{t('outcome.unverifiedNote')}</p>
        <div className="row">
          <button type="button" className="btn btn--primary grow" disabled={busy || warns.some((w) => w.key === 'outcome.warn.notAfter')} onClick={() => void save()}>
            {busy ? t('common.saving') : t('outcome.confirmSave')}
          </button>
          <button type="button" className="btn grow" onClick={() => setStep('form')} disabled={busy}>
            {t('outcome.change')}
          </button>
        </div>
      </div>
    );
  }

  return (
    <form className="form" onSubmit={toConfirm} noValidate>
      <div>
        <h1>{t('outcome.title', { caseNo })}</h1>
        <p className="muted">
          {name(d.court.name)} · {date(d.date)}
        </p>
      </div>
      {formError ? <Notice kind="error">{formError}</Notice> : null}

      <fieldset className="field">
        <legend>{tl('outcome.whatHappened')}</legend>
        <div className="choice-grid">
          {(types ?? []).map((ty) => (
            <label className="choice" key={ty.id}>
              <input type="radio" name="outcome-type" checked={f.outcomeTypeId === ty.id} onChange={() => chooseType(ty)} />
              {name(ty.name)}
            </label>
          ))}
        </div>
      </fieldset>

      <Field label={tl('outcome.orderText')} hint={t('outcome.orderTextHint')}>
        {(p) => <textarea {...p} rows={3} value={f.orderText} onChange={(e) => set('orderText', e.target.value)} />}
      </Field>

      <fieldset className="field">
        <legend>{tl('outcome.nextStep')}</legend>
        <div className="choice-list">
          <label className="choice">
            <input type="radio" name="has-next" checked={f.hasNext} onChange={() => set('hasNext', true)} />
            {t('outcome.hasNext')}
          </label>
          <label className="choice">
            <input
              type="radio"
              name="has-next"
              checked={!f.hasNext}
              onChange={() => setF((s) => ({ ...s, hasNext: false, noNextReason: s.noNextReason || (type ? defaultReason(type) : 'date_from_registry') }))}
            />
            {t('outcome.noNextDate')}
          </label>
        </div>
      </fieldset>

      {f.hasNext ? (
        <>
          <div className="grid-2">
            <Field label={tl('outcome.nextDate')} hint={isValidDate(f.nextDate) ? longDate(f.nextDate) : t('date.inputHint')}>
              {(p) => <input {...p} type="date" required min={d.date} value={f.nextDate} onChange={(e) => set('nextDate', e.target.value)} />}
            </Field>
            <Field label={tl('hearing.session')}>
              {(p) => (
                <select {...p} value={f.session} onChange={(e) => set('session', e.target.value as Session)}>
                  {SESSIONS.map((s) => (
                    <option key={s} value={s}>
                      {t(`session.${s}`)}
                    </option>
                  ))}
                </select>
              )}
            </Field>
          </div>
          <div className="grid-2">
            <Field label={tl('hearing.purpose')}>
              {(p) => (
                <select {...p} value={f.purposeId} onChange={(e) => set('purposeId', e.target.value)}>
                  <option value="">{t('common.choose')}</option>
                  {(purposes ?? []).map((x) => (
                    <option key={x.id} value={x.id}>
                      {name(x.name)}
                    </option>
                  ))}
                </select>
              )}
            </Field>
            <Field label={tl('hearing.time')} hint={t('hearing.timeHint')}>
              {(p) => <input {...p} type="time" value={f.time} onChange={(e) => set('time', e.target.value)} />}
            </Field>
          </div>
          <Field label={tl('hearing.purposeNote')} hint={t('hearing.purposeNoteHint')}>
            {(p) => <input {...p} type="text" value={f.purposeNote} onChange={(e) => set('purposeNote', e.target.value)} />}
          </Field>
        </>
      ) : (
        <>
          <fieldset className="field">
            <legend>{tl('outcome.noNextReason')}</legend>
            <div className="choice-list">
              {NO_NEXT_REASONS.map((r) => (
                <label className="choice" key={r}>
                  <input type="radio" name="no-next" checked={f.noNextReason === r} onChange={() => set('noNextReason', r)} />
                  {t(`outcome.noNext.${r}`)}
                </label>
              ))}
            </div>
          </fieldset>
          <Field label={tl('outcome.noNextNote')}>
            {(p) => <input {...p} type="text" value={f.noNextNote} onChange={(e) => set('noNextNote', e.target.value)} />}
          </Field>
        </>
      )}

      {appearing.length > 0 ? (
        <Field label={tl('outcome.whoAppeared')}>
          {(p) => (
            <select {...p} value={f.appearedUserId} onChange={(e) => set('appearedUserId', e.target.value)}>
              <option value="">{t('outcome.asListed')}</option>
              {appearing.map((a) => (
                <option key={a.user_id} value={a.user_id}>
                  {a.user_id === me.id ? t('outcome.me', { name: a.name }) : a.name}
                </option>
              ))}
            </select>
          )}
        </Field>
      ) : null}

      <div className="choice-list">
        <label className="choice">
          <input type="checkbox" checked={f.informClient} onChange={(e) => set('informClient', e.target.checked)} />
          {t('outcome.informClient')}
        </label>
        <label className="choice">
          <input type="checkbox" checked={f.certifiedCopy} onChange={(e) => set('certifiedCopy', e.target.checked)} />
          {t('outcome.certifiedCopy')}
        </label>
      </div>

      <button type="submit" className="btn btn--primary btn--block">
        {t('outcome.review')}
      </button>
    </form>
  );
}
