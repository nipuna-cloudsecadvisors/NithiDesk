import { useState } from 'react';
import { Link, useNavigate, useParams } from 'react-router-dom';
import { api, type Warning } from '../../api/client';
import { useUsers } from '../../api/reference';
import type { HearingDetail } from '../../api/types';
import { useAuth } from '../../auth/AuthProvider';
import { useT } from '../../i18n/I18nProvider';
import { colomboDateTime, compareDates, todayInColombo } from '../../lib/dates';
import { useQueuedFor } from '../../offline/QueueProvider';
import { ErrorNotice, Loading, Notice, Sheet, StatusTag, useAsync, useToast, VerificationBadge, Warnings } from '../../ui/components';
import { sessionLabel, usePartiesLine } from '../../ui/HearingEntry';
import { AssertDateForm, ListingStatusForm, NoteForm, PreparationForm, VacateForm, VerifyForm, type SimpleStatus } from './forms';

type Panel = null | 'note' | 'verify' | 'more' | 'assert' | 'vacate' | 'prep' | SimpleStatus;

export function useUserNames(): Record<string, string> {
  const users = useUsers();
  const { name } = useT();
  const out: Record<string, string> = {};
  for (const u of users ?? []) out[u.id] = name(u.display_name);
  return out;
}

export function Stamp({ at }: { at: string }) {
  const { date } = useT();
  const c = colomboDateTime(at);
  return <span>{c ? `${date(c.date)} ${c.time}` : at}</span>;
}

export function HearingScreen() {
  const { id = '' } = useParams();
  const { t, tl, name, otherName, date, longDate } = useT();
  const { can } = useAuth();
  const navigate = useNavigate();
  const toast = useToast();
  const names = useUserNames();
  const parties = usePartiesLine();
  const queued = useQueuedFor(id);
  const h = useAsync((signal) => api.get<HearingDetail>(`/api/v1/hearings/${id}`, { signal }), [id]);
  const [panel, setPanel] = useState<Panel>(null);
  const [warnings, setWarnings] = useState<Warning[]>([]);
  const [copied, setCopied] = useState(false);

  if (h.loading && !h.data) return <Loading />;
  if (!h.data) return <ErrorNotice error={h.error} />;
  const d = h.data;
  const today = todayInColombo();
  const scheduled = d.listing_status === 'scheduled';
  const outcomeOpen = ['scheduled', 'held', 'not_reached', 'court_not_sitting'].includes(d.listing_status);
  const canRecord = can('hearing.outcome') && outcomeOpen && compareDates(d.date, today) <= 0;
  const caseNo = d.case_number ?? t('hearing.noCaseNumber');
  const done = (next: HearingDetail | null, w?: Warning[]) => {
    setPanel(null);
    if (next) h.setData(next);
    else h.reload();
    setWarnings(w ?? []);
    toast('ok', t('common.saved'));
  };
  const copy = async () => {
    try {
      await navigator.clipboard.writeText(caseNo);
      setCopied(true);
      window.setTimeout(() => setCopied(false), 2000);
    } catch {
      // clipboard blocked: the number is selectable text anyway
    }
  };
  const purpose = d.purpose ? name(d.purpose.name) : '';
  const lastOutcome = d.last_outcome;
  const outcomes = d.outcomes ?? [];
  const notes = d.court_notes ?? [];
  const assertions = d.date_assertions ?? [];
  const verifications = d.verifications ?? [];

  const panelTitle: Record<Exclude<Panel, null>, string> = {
    note: t('note.title'),
    verify: t('verify.title'),
    more: t('hearing.moreActions'),
    assert: t('assert.title'),
    vacate: t('vacate.title'),
    prep: t('prep.title'),
    held: t('listingAction.held.title'),
    not_reached: t('listingAction.not_reached.title'),
    court_not_sitting: t('listingAction.court_not_sitting.title'),
    cancelled: t('listingAction.cancelled.title'),
  };

  return (
    <>
      <header className="case-head">
        <div className="case-head__no">
          <h1>{caseNo}</h1>
          {d.case_number ? (
            <button type="button" className="btn btn--quiet btn--small" onClick={() => void copy()}>
              {copied ? t('common.copied') : t('common.copy')}
            </button>
          ) : null}
          <VerificationBadge state={d.verification_state} overdue={d.verification_overdue} />
        </div>
        <p className="case-head__court">
          {[name(d.court.name), d.courtroom ? t('hearing.room', { room: d.courtroom }) : '', sessionLabel(t, d)].filter(Boolean).join(' · ')}
        </p>
        <p>
          <strong>{longDate(d.date)}</strong>
          <span className="muted">{` · ${date(d.date)}`}</span>
        </p>
      </header>

      {d.verification_state === 'conflict' ? (
        <div className="section">
          <Notice kind="error">{t('hearing.conflictExplain')}</Notice>
        </div>
      ) : null}
      {queued.length > 0 ? (
        <div className="section">
          <Notice kind="warn">
            {t('hearing.queuedHere', { count: queued.length })} <Link to="/outbox">{t('queue.open')}</Link>
          </Notice>
        </div>
      ) : null}
      <div className="section">
        <Warnings warnings={warnings} names={names} />
      </div>

      <section className="objective" aria-label={t('hearing.objective')}>
        <p className="objective__purpose">{[purpose, d.purpose_note].filter(Boolean).join(' — ') || t('hearing.noPurpose')}</p>
        {d.instructions ? (
          <p className="entry__instructions">
            <strong>{tl('hearing.instructions')}</strong>
            {': '}
            {d.instructions}
          </p>
        ) : null}
      </section>

      <section className="section">
        <h2>{tl('hearing.clientAndParties')}</h2>
        {d.title_masked ? (
          <p className="muted">{t('matter.titleMasked')}</p>
        ) : (
          <dl className="kv">
            <dt>{tl('party.ourClient')}</dt>
            <dd>
              {(d.our_clients ?? []).length === 0 ? <span className="muted">{t('common.none')}</span> : null}
              {(d.our_clients ?? []).map((c, i) => (
                <span key={i} className="stack--tight">
                  {name(c)}
                  {otherName(c) ? <span className="muted">{` (${otherName(c)})`}</span> : null}
                  <br />
                </span>
              ))}
            </dd>
            <dt>{tl('party.opposing')}</dt>
            <dd>
              {(d.opposing ?? []).length === 0 ? <span className="muted">{t('common.none')}</span> : null}
              {(d.opposing ?? []).map((c, i) => (
                <span key={i}>
                  {name(c)}
                  {otherName(c) ? <span className="muted">{` (${otherName(c)})`}</span> : null}
                  <br />
                </span>
              ))}
            </dd>
            <dt>{tl('matter.label')}</dt>
            <dd>
              <Link to={`/matters/${d.matter_id}`}>{d.matter_number}</Link>
              {' · '}
              {name(d.matter_title) || parties(d.our_clients, d.opposing)}
            </dd>
          </dl>
        )}
      </section>

      <section className="section">
        <h2>{tl('hearing.status')}</h2>
        <dl className="kv">
          <dt>{tl('hearing.listingStatus')}</dt>
          <dd>
            {t(`listing.${d.listing_status}`)}
            {d.status_reason ? <span className="muted">{` — ${d.status_reason}`}</span> : null}
          </dd>
          <dt>{tl('hearing.preparation')}</dt>
          <dd>
            <StatusTag kind={d.preparation_status === 'ready' ? 'ok' : d.preparation_status === 'blocked' ? 'bad' : 'wait'}>
              {tl(`preparation.${d.preparation_status}`)}
            </StatusTag>
            {d.preparation_note ? <span className="muted">{` ${d.preparation_note}`}</span> : null}
          </dd>
          <dt>{tl('hearing.appearing')}</dt>
          <dd>{(d.appearing ?? []).map((p) => p.name).join(', ') || <span className="tag tag--wait">{t('hearing.noAppearingLawyer')}</span>}</dd>
          {d.assigned_clerk ? (
            <>
              <dt>{tl('hearing.clerk')}</dt>
              <dd>{d.assigned_clerk.name}</dd>
            </>
          ) : null}
        </dl>
        <p className="row small section">
          {d.previous_hearing_id ? <Link to={`/hearings/${d.previous_hearing_id}`}>{t('hearing.previous')}</Link> : null}
          {d.next_hearing_id ? <Link to={`/hearings/${d.next_hearing_id}`}>{t('hearing.next')}</Link> : null}
          {d.replaced_by_hearing_id ? <Link to={`/hearings/${d.replaced_by_hearing_id}`}>{t('hearing.replacement')}</Link> : null}
          {d.replaces_hearing_id ? <Link to={`/hearings/${d.replaces_hearing_id}`}>{t('hearing.replaces')}</Link> : null}
        </p>
      </section>

      {lastOutcome ? (
        <section className="section">
          <h2>{tl('hearing.lastHearing')}</h2>
          <p>
            <Link to={`/hearings/${lastOutcome.hearing_id}`}>{date(lastOutcome.date)}</Link>
            {' — '}
            <strong>{name(lastOutcome.outcome_name)}</strong>
          </p>
          {lastOutcome.order_text ? <p className="pre">{lastOutcome.order_text}</p> : null}
          {lastOutcome.summary ? <p className="pre muted">{lastOutcome.summary}</p> : null}
        </section>
      ) : null}

      {outcomes.length > 0 ? (
        <section className="section">
          <h2>{tl('outcome.recorded')}</h2>
          <ul className="list">
            {outcomes.map((o) => (
              <li key={o.id}>
                <p className="list__title">
                  {name(o.outcome_name)}
                  {o.corrects_id ? <span className="tag">{` ${t('outcome.correction')}`}</span> : null}
                </p>
                {o.order_text ? <p className="pre">{o.order_text}</p> : null}
                {o.no_next_reason ? <p className="muted">{t(`outcome.noNext.${o.no_next_reason}`)}</p> : null}
                <p className="small muted">
                  {t('common.byAt', { who: o.recorded_by })} <Stamp at={o.recorded_at} />
                </p>
              </li>
            ))}
          </ul>
        </section>
      ) : null}

      <section className="section">
        <h2>{tl('note.notes')}</h2>
        {d.notes ? <p className="pre panel">{d.notes}</p> : null}
        {notes.length === 0 && !d.notes ? <p className="muted">{t('note.none')}</p> : null}
        {notes.length > 0 ? (
          <ul className="list">
            {notes.map((n) => (
              <li key={n.id}>
                <p className="pre">{n.body}</p>
                <p className="small muted">
                  {t('common.byAt', { who: n.created_by })} <Stamp at={n.created_at} />
                </p>
              </li>
            ))}
          </ul>
        ) : null}
      </section>

      <section className="section">
        <h2>{tl('hearing.dateHistory')}</h2>
        <ol className="timeline">
          {assertions.map((a) => {
            const ver = verifications.filter((v) => v.date_assertion_id === a.id);
            return (
              <li key={a.id} className={a.current ? 'is-current' : undefined}>
                <p className="timeline__date">
                  {longDate(a.date)}
                  {a.session !== 'unspecified' ? ` · ${t(`session.${a.session}`)}` : ''}
                  {a.time ? ` · ${a.time}` : ''}
                  {a.current ? <span className="tag">{` ${t('assertion.current')}`}</span> : null}
                </p>
                <p className="small">
                  {name(a.source_name)} · {t(`authority.${a.authority}`)} · {t(`assertion.kind.${a.kind}`)}
                  {a.source_detail ? ` · ${a.source_detail}` : ''}
                </p>
                <p className="small muted">
                  {t('common.byAt', { who: a.asserted_by ?? t('common.system') })} <Stamp at={a.asserted_at} />
                </p>
                {ver.map((v) => (
                  <p key={v.id} className="small">
                    {v.revoked_at ? (
                      <span className="tag tag--bad">{t('verification.revoked', { reason: v.revoke_reason ?? '' })}</span>
                    ) : (
                      <span className="tag tag--ok">
                        {t('verification.byLine', { who: v.verified_by, evidence: v.evidence_reference })}
                      </span>
                    )}{' '}
                    <Stamp at={v.verified_at} />
                  </p>
                ))}
              </li>
            );
          })}
        </ol>
      </section>

      <div className="actionbar no-print" role="toolbar" aria-label={t('hearing.actions')}>
        {canRecord ? (
          <button type="button" className="btn btn--primary" onClick={() => navigate(`/hearings/${d.id}/outcome`)}>
            {t('hearing.recordOutcome')}
          </button>
        ) : null}
        {d.access === 'full' ? (
          <button type="button" className="btn" onClick={() => setPanel('note')}>
            {t('note.add')}
          </button>
        ) : null}
        {can('hearing.verify') && scheduled && d.verification_state !== 'verified' ? (
          <button type="button" className="btn" onClick={() => setPanel('verify')}>
            {t('verify.action')}
          </button>
        ) : null}
        <button type="button" className="btn" onClick={() => setPanel('more')}>
          {t('hearing.more')}
        </button>
      </div>

      <Sheet open={panel !== null} title={panel ? panelTitle[panel] : ''} onClose={() => setPanel(null)}>
        {panel === 'note' ? <NoteForm hearing={d} onDone={() => setPanel(null)} /> : null}
        {panel === 'verify' ? <VerifyForm hearing={d} onDone={done} /> : null}
        {panel === 'assert' ? <AssertDateForm hearing={d} onDone={done} /> : null}
        {panel === 'vacate' ? <VacateForm hearing={d} onDone={done} /> : null}
        {panel === 'prep' ? <PreparationForm hearing={d} onDone={done} /> : null}
        {panel === 'held' || panel === 'not_reached' || panel === 'court_not_sitting' || panel === 'cancelled' ? (
          <ListingStatusForm hearing={d} status={panel} onDone={done} />
        ) : null}
        {panel === 'more' ? (
          <div className="menu">
            {scheduled && can('hearing.write') && d.access === 'full' ? (
              <button type="button" className="btn" onClick={() => setPanel('prep')}>
                {t('prep.title')}
              </button>
            ) : null}
            {scheduled && can('hearing.write') ? (
              <>
                <button type="button" className="btn" onClick={() => setPanel('vacate')}>
                  {t('more.courtChangedDate')}
                </button>
                <button type="button" className="btn" onClick={() => setPanel('assert')}>
                  {t('more.recordedWrongly')}
                </button>
                <button type="button" className="btn" onClick={() => setPanel('held')}>
                  {t('more.markHeld')}
                </button>
                <button type="button" className="btn" onClick={() => setPanel('not_reached')}>
                  {t('more.notReached')}
                </button>
                <button type="button" className="btn" onClick={() => setPanel('court_not_sitting')}>
                  {t('more.courtNotSitting')}
                </button>
                <button type="button" className="btn btn--danger" onClick={() => setPanel('cancelled')}>
                  {t('more.cancel')}
                </button>
              </>
            ) : null}
            {!scheduled && can('hearing.write') ? (
              <button type="button" className="btn" onClick={() => setPanel('assert')}>
                {t('more.recordedWrongly')}
              </button>
            ) : null}
            <Link className="btn" to={`/matters/${d.matter_id}`}>
              {t('more.openMatter')}
            </Link>
          </div>
        ) : null}
      </Sheet>
    </>
  );
}
