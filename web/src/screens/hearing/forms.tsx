import { useMemo, useState, type FormEvent } from 'react';
import { api, ApiError, type Warning, type WithWarnings } from '../../api/client';
import { authoritativeSources, useReference } from '../../api/reference';
import type { DateClaim, HearingDetail, Session } from '../../api/types';
import { useT } from '../../i18n/I18nProvider';
import { isValidDate } from '../../lib/dates';
import { uuidv4 } from '../../lib/uuid';
import { useQueue } from '../../offline/QueueProvider';
import { ErrorNotice, Field, Notice, useToast } from '../../ui/components';

export const SESSIONS: Session[] = ['unspecified', 'morning', 'afternoon'];

export function currentAssertion(h: HearingDetail) {
  return (h.date_assertions ?? []).find((a) => a.current) ?? (h.date_assertions ?? [])[0];
}

/** Date + session + time + source: one claim about when the hearing is. */
export function DateClaimFields({
  value,
  onChange,
  error,
  sourceFilter,
}: {
  value: DateClaim;
  onChange: (v: DateClaim) => void;
  error?: unknown;
  sourceFilter?: (code: string, authority: string) => boolean;
}) {
  const { t, tl, name, fieldError, longDate } = useT();
  const sources = useReference('date-source-types');
  const shown = (sources ?? []).filter((s) => !sourceFilter || sourceFilter(s.code, String(s.attributes?.default_authority ?? '')));
  return (
    <>
      <div className="grid-2">
        <Field
          label={tl('hearing.date')}
          hint={isValidDate(value.date) ? longDate(value.date) : t('date.inputHint')}
          error={fieldError(error, 'date') ?? fieldError(error, 'replacement.date')}
        >
          {(p) => <input {...p} type="date" required value={value.date} onChange={(e) => onChange({ ...value, date: e.target.value })} />}
        </Field>
        <Field label={tl('hearing.session')}>
          {(p) => (
            <select {...p} value={value.session} onChange={(e) => onChange({ ...value, session: e.target.value as Session })}>
              {SESSIONS.map((s) => (
                <option key={s} value={s}>
                  {t(`session.${s}`)}
                </option>
              ))}
            </select>
          )}
        </Field>
      </div>
      <Field label={tl('hearing.time')} hint={t('hearing.timeHint')} error={fieldError(error, 'time')}>
        {(p) => <input {...p} type="time" value={value.time ?? ''} onChange={(e) => onChange({ ...value, time: e.target.value })} />}
      </Field>
      <Field label={tl('hearing.dateSource')} hint={t('hearing.dateSourceHint')} error={fieldError(error, 'source_type_id')}>
        {(p) => (
          <select {...p} required value={value.source_type_id} onChange={(e) => onChange({ ...value, source_type_id: e.target.value })}>
            <option value="">{t('common.choose')}</option>
            {shown.map((s) => (
              <option key={s.id} value={s.id}>
                {name(s.name)} — {t(`authority.${String(s.attributes?.default_authority ?? 'hearsay')}`)}
              </option>
            ))}
          </select>
        )}
      </Field>
      <Field label={tl('hearing.sourceDetail')} hint={t('hearing.sourceDetailHint')}>
        {(p) => <input {...p} type="text" value={value.source_detail ?? ''} onChange={(e) => onChange({ ...value, source_detail: e.target.value })} />}
      </Field>
    </>
  );
}

export function emptyClaim(date = '', sourceId = ''): DateClaim {
  return { date, session: 'unspecified', time: '', source_type_id: sourceId, source_detail: '' };
}

export function cleanClaim(c: DateClaim): DateClaim {
  const out: DateClaim = { date: c.date, session: c.session, source_type_id: c.source_type_id };
  if (c.time) out.time = c.time;
  if (c.source_detail?.trim()) out.source_detail = c.source_detail.trim();
  return out;
}

type Done = (d: HearingDetail | null, warnings?: Warning[]) => void;

// ---------------------------------------------------------------------------

export function NoteForm({ hearing, onDone }: { hearing: HearingDetail; onDone: () => void }) {
  const { t, tl } = useT();
  const { submit } = useQueue();
  const toast = useToast();
  const [body, setBody] = useState('');
  const [busy, setBusy] = useState(false);
  const save = async (e: FormEvent) => {
    e.preventDefault();
    if (!body.trim()) return;
    setBusy(true);
    try {
      const key = uuidv4();
      const left = await submit({
        key,
        kind: 'note',
        hearingId: hearing.id,
        method: 'POST',
        path: `/api/v1/hearings/${hearing.id}/notes`,
        body: { body: body.trim(), client_recorded_at: new Date().toISOString() },
        label: { caseNumber: hearing.case_number, courtDate: hearing.date },
      });
      toast(left ? 'warn' : 'ok', left ? t('queue.savedWaiting') : t('note.saved'));
      onDone();
    } finally {
      setBusy(false);
    }
  };
  return (
    <form className="form" onSubmit={save}>
      <Field label={tl('note.label')} hint={t('note.hint')}>
        {(p) => <textarea {...p} value={body} onChange={(e) => setBody(e.target.value)} rows={5} autoFocus />}
      </Field>
      <div className="row row--end">
        <button type="submit" className="btn btn--primary" disabled={busy || !body.trim()}>
          {t('note.save')}
        </button>
      </div>
    </form>
  );
}

// ---------------------------------------------------------------------------

export function VerifyForm({ hearing, onDone }: { hearing: HearingDetail; onDone: Done }) {
  const { t, tl, name, longDate } = useT();
  const sources = authoritativeSources(useReference('date-source-types'));
  const current = currentAssertion(hearing);
  const [sourceId, setSourceId] = useState('');
  const [evidence, setEvidence] = useState('');
  const [error, setError] = useState<unknown>(null);
  const [busy, setBusy] = useState(false);
  const chosen = sourceId || sources[0]?.id || '';
  const save = async (e: FormEvent) => {
    e.preventDefault();
    if (!current) return;
    setBusy(true);
    setError(null);
    try {
      const r = await api.post<WithWarnings<HearingDetail>>(`/api/v1/hearings/${hearing.id}/verifications`, {
        date_assertion_id: current.id,
        source_type_id: chosen,
        evidence_reference: evidence.trim(),
      });
      onDone(r.data, r.warnings);
    } catch (err) {
      setError(err);
    } finally {
      setBusy(false);
    }
  };
  if (!current) return <Notice kind="warn">{t('verify.noAssertion')}</Notice>;
  return (
    <form className="form" onSubmit={save}>
      <p>{t('verify.confirming', { date: longDate(current.date) })}</p>
      <ErrorNotice error={error} />
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
        {(p) => <input {...p} type="text" required value={evidence} onChange={(e) => setEvidence(e.target.value)} autoFocus />}
      </Field>
      <div className="row row--end">
        <button type="submit" className="btn btn--primary" disabled={busy || !evidence.trim() || !chosen}>
          {t('verify.submit')}
        </button>
      </div>
    </form>
  );
}

// ---------------------------------------------------------------------------

/** "The date was recorded wrongly": a correcting assertion on this hearing. */
export function AssertDateForm({ hearing, onDone, defaultSourceCode }: { hearing: HearingDetail; onDone: Done; defaultSourceCode?: string }) {
  const { t } = useT();
  const sources = useReference('date-source-types');
  const defaultSource = sources?.find((s) => s.code === (defaultSourceCode ?? 'case_record'))?.id ?? '';
  const [claim, setClaim] = useState<DateClaim>(() => emptyClaim(hearing.date));
  const key = useMemo(() => uuidv4(), []);
  const [error, setError] = useState<unknown>(null);
  const [busy, setBusy] = useState(false);
  const value = { ...claim, source_type_id: claim.source_type_id || defaultSource };
  const save = async (e: FormEvent) => {
    e.preventDefault();
    setBusy(true);
    setError(null);
    try {
      const r = await api.post<WithWarnings<HearingDetail>>(`/api/v1/hearings/${hearing.id}/date-assertions`, cleanClaim(value), {
        idempotencyKey: key,
      });
      onDone(r.data, r.warnings);
    } catch (err) {
      setError(err);
    } finally {
      setBusy(false);
    }
  };
  return (
    <form className="form" onSubmit={save}>
      <p>{t('assert.body')}</p>
      <ErrorNotice error={error} />
      <DateClaimFields value={value} onChange={setClaim} error={error} />
      <div className="row row--end">
        <button type="submit" className="btn btn--primary" disabled={busy || !isValidDate(value.date) || !value.source_type_id}>
          {t('assert.submit')}
        </button>
      </div>
    </form>
  );
}

// ---------------------------------------------------------------------------

/** "The court changed the date before the hearing": vacate + replacement. */
export function VacateForm({ hearing, onDone }: { hearing: HearingDetail; onDone: Done }) {
  const { t, tl, fieldError } = useT();
  const sources = useReference('date-source-types');
  const [reason, setReason] = useState('');
  const [withReplacement, setWithReplacement] = useState(true);
  const [claim, setClaim] = useState<DateClaim>(() => emptyClaim(''));
  const [error, setError] = useState<unknown>(null);
  const [busy, setBusy] = useState(false);
  const defaultSource = sources?.find((s) => s.code === 'court_notice')?.id ?? '';
  const value = { ...claim, source_type_id: claim.source_type_id || defaultSource };
  const save = async (e: FormEvent) => {
    e.preventDefault();
    setBusy(true);
    setError(null);
    try {
      const body: Record<string, unknown> = { reason: reason.trim() };
      if (withReplacement) body.replacement = cleanClaim(value);
      const r = await api.post<WithWarnings<HearingDetail>>(`/api/v1/hearings/${hearing.id}/vacate`, body);
      onDone(r.data, r.warnings);
    } catch (err) {
      setError(err);
    } finally {
      setBusy(false);
    }
  };
  return (
    <form className="form" onSubmit={save}>
      <p>{t('vacate.body')}</p>
      <ErrorNotice error={error} />
      <Field label={tl('vacate.reason')} error={fieldError(error, 'reason')}>
        {(p) => <input {...p} type="text" required value={reason} onChange={(e) => setReason(e.target.value)} />}
      </Field>
      <label className="choice">
        <input type="checkbox" checked={withReplacement} onChange={(e) => setWithReplacement(e.target.checked)} />
        {t('vacate.hasNewDate')}
      </label>
      {withReplacement ? <DateClaimFields value={value} onChange={setClaim} error={error} /> : null}
      <div className="row row--end">
        <button
          type="submit"
          className="btn btn--primary"
          disabled={busy || !reason.trim() || (withReplacement && (!isValidDate(value.date) || !value.source_type_id))}
        >
          {t('vacate.submit')}
        </button>
      </div>
    </form>
  );
}

// ---------------------------------------------------------------------------

export type SimpleStatus = 'held' | 'not_reached' | 'court_not_sitting' | 'cancelled';

export function ListingStatusForm({ hearing, status, onDone }: { hearing: HearingDetail; status: SimpleStatus; onDone: Done }) {
  const { t, tl, fieldError } = useT();
  const [reason, setReason] = useState('');
  const [error, setError] = useState<unknown>(null);
  const [busy, setBusy] = useState(false);
  const needsReason = status === 'cancelled';
  const save = async (e: FormEvent) => {
    e.preventDefault();
    setBusy(true);
    setError(null);
    try {
      const r = await api.post<WithWarnings<HearingDetail>>(`/api/v1/hearings/${hearing.id}/listing-status`, {
        status,
        reason: reason.trim(),
      });
      onDone(r.data, r.warnings);
    } catch (err) {
      setError(err);
    } finally {
      setBusy(false);
    }
  };
  return (
    <form className="form" onSubmit={save}>
      <p>{t(`listingAction.${status}.body`)}</p>
      <ErrorNotice error={error} />
      <Field label={needsReason ? tl('listingAction.reasonRequired') : tl('listingAction.reasonOptional')} error={fieldError(error, 'reason')}>
        {(p) => <input {...p} type="text" required={needsReason} value={reason} onChange={(e) => setReason(e.target.value)} />}
      </Field>
      <div className="row row--end">
        <button type="submit" className={status === 'cancelled' ? 'btn btn--danger' : 'btn btn--primary'} disabled={busy || (needsReason && !reason.trim())}>
          {t(`listingAction.${status}.submit`)}
        </button>
      </div>
    </form>
  );
}

// ---------------------------------------------------------------------------

const PREP = ['not_started', 'in_progress', 'ready', 'blocked'] as const;

/** PATCH with If-Match: a 412 reloads the hearing and says someone else changed it. */
export function PreparationForm({ hearing, onDone }: { hearing: HearingDetail; onDone: Done }) {
  const { t, tl, fieldError } = useT();
  const [base, setBase] = useState(hearing);
  const [status, setStatus] = useState(hearing.preparation_status);
  const [note, setNote] = useState(hearing.preparation_note ?? '');
  const [instructions, setInstructions] = useState(hearing.instructions ?? '');
  const [error, setError] = useState<unknown>(null);
  const [stale, setStale] = useState(false);
  const [busy, setBusy] = useState(false);
  const save = async (e: FormEvent) => {
    e.preventDefault();
    setBusy(true);
    setError(null);
    setStale(false);
    try {
      const r = await api.patch<WithWarnings<HearingDetail>>(
        `/api/v1/hearings/${hearing.id}`,
        { preparation_status: status, preparation_note: note.trim(), instructions: instructions.trim() },
        base.row_version,
      );
      onDone(r.data, r.warnings);
    } catch (err) {
      if (err instanceof ApiError && err.status === 412) {
        const fresh = await api.get<HearingDetail>(`/api/v1/hearings/${hearing.id}`).catch(() => null);
        if (fresh) {
          setBase(fresh);
          setStatus(fresh.preparation_status);
          setNote(fresh.preparation_note ?? '');
          setInstructions(fresh.instructions ?? '');
        }
        setStale(true);
      } else {
        setError(err);
      }
    } finally {
      setBusy(false);
    }
  };
  return (
    <form className="form" onSubmit={save}>
      {stale ? <Notice kind="warn">{t('error.precondition.version_mismatch')}</Notice> : null}
      <ErrorNotice error={error} />
      <Field label={tl('hearing.preparation')} error={fieldError(error, 'preparation_status')}>
        {(p) => (
          <select {...p} value={status} onChange={(e) => setStatus(e.target.value)}>
            {PREP.map((x) => (
              <option key={x} value={x}>
                {t(`preparation.${x}`)}
              </option>
            ))}
          </select>
        )}
      </Field>
      <Field label={tl('prep.note')} hint={t('prep.noteHint')}>
        {(p) => <input {...p} type="text" value={note} onChange={(e) => setNote(e.target.value)} />}
      </Field>
      <Field label={tl('hearing.instructions')}>
        {(p) => <textarea {...p} rows={4} value={instructions} onChange={(e) => setInstructions(e.target.value)} />}
      </Field>
      <div className="row row--end">
        <button type="submit" className="btn btn--primary" disabled={busy}>
          {t('prep.save')}
        </button>
      </div>
    </form>
  );
}
