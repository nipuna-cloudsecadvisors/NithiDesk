import { useEffect, useMemo, useState, type FormEvent } from 'react';
import { Link, useLocation, useNavigate, useParams } from 'react-router-dom';
import { api, type Warning, type WithWarnings } from '../../api/client';
import { useCourts, useReference } from '../../api/reference';
import type { Card, DateClaim, HearingDetail, MatterDetail, Proceeding } from '../../api/types';
import { useAuth } from '../../auth/AuthProvider';
import { useT } from '../../i18n/I18nProvider';
import { isValidDate } from '../../lib/dates';
import { uuidv4 } from '../../lib/uuid';
import { ErrorNotice, Field, Loading, Notice, Sheet, useAsync, useToast, VerificationBadge, Warnings } from '../../ui/components';
import { sessionLabel } from '../../ui/HearingEntry';
import { cleanClaim, DateClaimFields, emptyClaim } from '../hearing/forms';
import { useUserNames } from '../hearing/HearingScreen';
import { EntityPicker, entityPayload, type EntityChoice } from './EntityPicker';

type Panel = null | { kind: 'proceeding' } | { kind: 'hearing' | 'caseNumber' | 'link' | 'party'; proceeding: Proceeding };

export function MatterPage() {
  const { id = '' } = useParams();
  const { t, tl, name, otherName } = useT();
  const { can } = useAuth();
  const toast = useToast();
  const location = useLocation();
  const names = useUserNames();
  const m = useAsync((signal) => api.get<MatterDetail>(`/api/v1/matters/${id}`, { signal }), [id]);
  const procs = useAsync(
    (signal) => api.get<{ items: Proceeding[] | null }>(`/api/v1/matters/${id}/proceedings`, { signal }).then((r) => r.items ?? []),
    [id],
  );
  const [panel, setPanel] = useState<Panel>(null);
  const [warnings, setWarnings] = useState<Warning[]>([]);

  useEffect(() => {
    if (location.hash && procs.data) document.getElementById(location.hash.slice(1))?.scrollIntoView();
  }, [location.hash, procs.data]);

  if (m.loading && !m.data) return <Loading />;
  if (!m.data) return <ErrorNotice error={m.error} />;
  const d = m.data;
  const full = d.access === 'full';
  const done = (w?: Warning[]) => {
    setPanel(null);
    setWarnings(w ?? []);
    toast('ok', t('common.saved'));
    procs.reload();
    m.reload();
  };
  const proceedings = procs.data ?? [];

  return (
    <>
      <header className="case-head">
        <div className="case-head__no">
          <h1>{d.matter_number}</h1>
          <span className="tag">{t(`lifecycle.${d.lifecycle}`)}</span>
          {d.priority !== 'normal' ? <span className="tag tag--wait">{t(`priority.${d.priority}`)}</span> : null}
          {d.confidentiality !== 'standard' ? <span className="tag tag--bad">{t(`confidentiality.${d.confidentiality}`)}</span> : null}
        </div>
        <p className="case-head__court">
          {d.title_masked ? t('matter.titleMasked') : name(d.title)}
          {!d.title_masked && otherName(d.title) ? <span className="muted">{` / ${otherName(d.title)}`}</span> : null}
        </p>
      </header>

      {(d.attention_flags ?? []).length > 0 ? (
        <ul className="badges section">
          {(d.attention_flags ?? []).map((f) => (
            <li key={f} className={f === 'hearing_within_7_days' ? 'badge' : 'badge badge--wait'}>
              <span aria-hidden="true">{f === 'hearing_within_7_days' ? '▤' : '!'}</span>
              {t(`flag.${f}`)}
            </li>
          ))}
        </ul>
      ) : null}
      <Warnings warnings={warnings} names={names} />

      <section className="section grid-2">
        <div>
          <h2 className="section__title">{tl('party.clients')}</h2>
          <ul className="list">
            {(d.clients ?? []).map((c) => (
              <li key={c.entity_id}>
                {name(c.name)}
                {otherName(c.name) ? <span className="muted">{` / ${otherName(c.name)}`}</span> : null}
              </li>
            ))}
          </ul>
        </div>
        <div>
          <h2 className="section__title">{tl('matter.team')}</h2>
          <ul className="list">
            {(d.team ?? []).map((tm) => (
              <li key={tm.id}>
                {tm.display_name} <span className="muted">{`— ${t(`teamRole.${tm.team_role}`)}`}</span>
              </li>
            ))}
          </ul>
        </div>
      </section>
      {d.description ? <p className="section pre">{d.description}</p> : null}

      <section className="section">
        <div className="page-head">
          <h2 className="page-head__title">{tl('proceeding.plural')}</h2>
          {full && can('proceeding.write') ? (
            <button type="button" className="btn btn--primary" onClick={() => setPanel({ kind: 'proceeding' })}>
              {t('proceeding.add')}
            </button>
          ) : null}
        </div>
        {procs.loading && !procs.data ? <Loading /> : null}
        <ErrorNotice error={procs.error} />
        {procs.data && proceedings.length === 0 ? <p className="muted">{t('proceeding.none')}</p> : null}
        <div className="stack">
          {proceedings.map((p) => (
            <ProceedingBlock
              key={p.id}
              p={p}
              all={proceedings}
              canWrite={full && can('proceeding.write')}
              canHearing={full && can('hearing.write')}
              onAction={(kind) => setPanel({ kind, proceeding: p })}
            />
          ))}
        </div>
      </section>

      <Sheet
        open={panel !== null}
        title={
          panel?.kind === 'proceeding'
            ? t('proceeding.add')
            : panel?.kind === 'hearing'
              ? t('hearingNew.title')
              : panel?.kind === 'caseNumber'
                ? t('caseNumber.add')
                : panel?.kind === 'party'
                  ? t('party.add')
                  : t('link.add')
        }
        onClose={() => setPanel(null)}
      >
        {panel?.kind === 'proceeding' ? <ProceedingForm matterId={d.id} onDone={done} /> : null}
        {panel?.kind === 'hearing' ? <HearingForm matter={d} proceeding={panel.proceeding} onDone={done} /> : null}
        {panel?.kind === 'caseNumber' ? <CaseNumberForm proceeding={panel.proceeding} onDone={done} /> : null}
        {panel?.kind === 'party' ? <PartyForm proceeding={panel.proceeding} onDone={done} /> : null}
        {panel?.kind === 'link' ? <LinkForm proceeding={panel.proceeding} all={proceedings} onDone={done} /> : null}
      </Sheet>
    </>
  );
}


function ProceedingBlock({
  p,
  all,
  canWrite,
  canHearing,
  onAction,
}: {
  p: Proceeding;
  all: Proceeding[];
  canWrite: boolean;
  canHearing: boolean;
  onAction: (k: 'hearing' | 'caseNumber' | 'link' | 'party') => void;
}) {
  const { t, tl, name, date } = useT();
  const courts = useCourts();
  const types = useReference('proceeding-types');
  const roles = useReference('party-role-types');
  const rels = useReference('relationship-types');
  const hs = useAsync(
    (signal) => api.get<{ items: Card[] | null }>(`/api/v1/proceedings/${p.id}/hearings`, { signal }).then((r) => r.items ?? []),
    [p.id, p.row_version],
  );
  const court = courts?.find((c) => c.id === p.court_id);
  const type = types?.find((x) => x.id === p.proceeding_type_id);
  return (
    <article className="panel" id={`p-${p.id}`}>
      <div className="row">
        <h3 className="grow">
          <span className="entry__caseno">{p.current_case_number ?? t('hearing.noCaseNumber')}</span>
        </h3>
        <span className="tag">{t(`proceedingStatus.${p.status}`)}</span>
      </div>
      <p>
        {[court ? name(court.name) : '', type ? name(type.name) : '', p.our_side ? t(`ourSide.${p.our_side}`) : '', t(`ourRole.${p.our_role}`)]
          .filter(Boolean)
          .join(' · ')}
      </p>
      {name(p.title) ? <p className="muted">{name(p.title)}</p> : null}

      <h4 className="section">{tl('party.plural')}</h4>
      <ul className="stack stack--tight small">
        {(p.parties ?? []).map((x) => (
          <li key={x.id}>
            {name(x.name)}
            <span className="muted">
              {` — ${name(roles?.find((r) => r.id === x.party_role_type_id)?.name) || x.party_role_code}${x.ordinal ? ` (${x.ordinal})` : ''} · ${t(`partySide.${x.side}`)}`}
            </span>
          </li>
        ))}
      </ul>

      {(p.case_numbers ?? []).length > 1 ? (
        <details className="section">
          <summary>{tl('caseNumber.history')}</summary>
          <ul className="small">
            {(p.case_numbers ?? []).map((c) => (
              <li key={c.id}>
                {c.raw} <span className="muted">{`— ${t(`caseNumber.reason.${c.reason}`)} · ${date(c.valid_from)}${c.valid_to ? `–${date(c.valid_to)}` : ''}`}</span>
              </li>
            ))}
          </ul>
        </details>
      ) : null}
      {(p.relationships ?? []).length > 0 ? (
        <details className="section">
          <summary>{tl('link.related')}</summary>
          <ul className="small">
            {(p.relationships ?? []).map((r) => {
              const rt = rels?.find((x) => x.code === r.relationship_code);
              const other = all.find((x) => x.id === r.other_proceeding_id);
              return (
                <li key={r.id}>
                  {rt ? name(rt.name) : r.relationship_code}
                  {r.direction !== 'outgoing' ? ` ${t('link.incoming')}` : ''} {r.other_case_number ?? other?.current_case_number ?? ''}
                </li>
              );
            })}
          </ul>
        </details>
      ) : null}

      <h4 className="section">{tl('hearing.plural')}</h4>
      {hs.loading && !hs.data ? <Loading /> : null}
      {hs.data && hs.data.length === 0 ? <p className="muted small">{t('hearing.none')}</p> : null}
      <ul className="week-rows">
        {(hs.data ?? []).map((c) => (
          <li key={c.id} className="row">
            <Link to={`/hearings/${c.id}`}>{date(c.date)}</Link>
            <VerificationBadge state={c.verification_state} />
            <span className="muted small">
              {[c.purpose ? name(c.purpose.name) : '', sessionLabel(t, c), t(`listing.${c.listing_status}`)].filter(Boolean).join(' · ')}
            </span>
          </li>
        ))}
      </ul>

      <div className="row section">
        {canHearing ? (
          <button type="button" className="btn btn--small btn--primary" onClick={() => onAction('hearing')}>
            {t('hearingNew.title')}
          </button>
        ) : null}
        {canWrite ? (
          <>
            <button type="button" className="btn btn--small" onClick={() => onAction('caseNumber')}>
              {t('caseNumber.add')}
            </button>
            <button type="button" className="btn btn--small" onClick={() => onAction('party')}>
              {t('party.add')}
            </button>
            <button type="button" className="btn btn--small" onClick={() => onAction('link')}>
              {t('link.add')}
            </button>
          </>
        ) : null}
      </div>
    </article>
  );
}

// ---------------------------------------------------------------------------

const OUR_SIDES = ['plaintiff_side', 'defendant_side', 'petitioner', 'respondent', 'appellant', 'accused', 'complainant', 'intervenient', 'other'];
const OUR_ROLES = ['attorney_on_record', 'counsel_only', 'watching_brief'];
const PARTY_SIDES = ['our_client', 'aligned', 'opposing', 'neutral'];

interface PartyDraft {
  entity: EntityChoice | null;
  roleTypeId: string;
  side: string;
  ordinal: string;
}

function PartyFields({ value, onChange, index }: { value: PartyDraft; onChange: (v: PartyDraft) => void; index: number }) {
  const { t, tl, name } = useT();
  const roles = useReference('party-role-types');
  return (
    <fieldset className="field panel">
      <legend>{t('party.numbered', { n: index + 1 })}</legend>
      <div className="stack">
        <EntityPicker label={tl('party.name')} value={value.entity} onChange={(e) => onChange({ ...value, entity: e })} />
        <div className="grid-2">
          <Field label={tl('party.role')}>
            {(p) => (
              <select {...p} value={value.roleTypeId} onChange={(e) => onChange({ ...value, roleTypeId: e.target.value })}>
                <option value="">{t('common.choose')}</option>
                {(roles ?? []).map((r) => (
                  <option key={r.id} value={r.id}>
                    {name(r.name)}
                  </option>
                ))}
              </select>
            )}
          </Field>
          <Field label={tl('party.side')}>
            {(p) => (
              <select {...p} value={value.side} onChange={(e) => onChange({ ...value, side: e.target.value })}>
                {PARTY_SIDES.map((s) => (
                  <option key={s} value={s}>
                    {t(`partySide.${s}`)}
                  </option>
                ))}
              </select>
            )}
          </Field>
          <Field label={tl('party.ordinal')} hint={t('party.ordinalHint')}>
            {(p) => <input {...p} type="number" min={1} value={value.ordinal} onChange={(e) => onChange({ ...value, ordinal: e.target.value })} />}
          </Field>
        </div>
      </div>
    </fieldset>
  );
}

function partyPayload(p: PartyDraft) {
  if (!p.entity || !p.roleTypeId) return null;
  const out: Record<string, unknown> = { ...entityPayload(p.entity), party_role_type_id: p.roleTypeId, side: p.side };
  if (p.ordinal) out.ordinal = Number(p.ordinal);
  return out;
}

const emptyParty = (side = 'our_client'): PartyDraft => ({ entity: null, roleTypeId: '', side, ordinal: '' });

function ProceedingForm({ matterId, onDone }: { matterId: string; onDone: (w?: Warning[]) => void }) {
  const { t, tl, name, fieldError } = useT();
  const courts = useCourts();
  const types = useReference('proceeding-types');
  const key = useMemo(() => uuidv4(), []);
  const [typeId, setTypeId] = useState('');
  const [courtId, setCourtId] = useState('');
  const [caseNo, setCaseNo] = useState('');
  const [titleEn, setTitleEn] = useState('');
  const [titleSi, setTitleSi] = useState('');
  const [ourSide, setOurSide] = useState('');
  const [ourRole, setOurRole] = useState('attorney_on_record');
  const [instituted, setInstituted] = useState('');
  const [parties, setParties] = useState<PartyDraft[]>([emptyParty('our_client'), emptyParty('opposing')]);
  const [error, setError] = useState<unknown>(null);
  const [busy, setBusy] = useState(false);
  const court = courts?.find((c) => c.id === courtId);
  const example = court?.case_number_patterns?.[0]?.example;

  const submit = async (e: FormEvent) => {
    e.preventDefault();
    setBusy(true);
    setError(null);
    try {
      const body: Record<string, unknown> = {
        proceeding_type_id: typeId,
        court_id: courtId,
        our_role: ourRole,
        parties: parties.map(partyPayload).filter(Boolean),
      };
      if (caseNo.trim()) body.case_number = caseNo.trim();
      if (titleEn.trim()) body.title_en = titleEn.trim();
      if (titleSi.trim()) body.title_si = titleSi.trim();
      if (ourSide) body.our_side = ourSide;
      if (instituted) body.instituted_on = instituted;
      const r = await api.post<WithWarnings<Proceeding>>(`/api/v1/matters/${matterId}/proceedings`, body, { idempotencyKey: key });
      onDone(r.warnings);
    } catch (err) {
      setError(err);
    } finally {
      setBusy(false);
    }
  };

  return (
    <form className="form" onSubmit={submit} noValidate>
      <ErrorNotice error={error} />
      <Field label={tl('proceeding.type')} error={fieldError(error, 'proceeding_type_id')}>
        {(p) => (
          <select {...p} required value={typeId} onChange={(e) => setTypeId(e.target.value)}>
            <option value="">{t('common.choose')}</option>
            {(types ?? []).map((x) => (
              <option key={x.id} value={x.id}>
                {name(x.name)}
              </option>
            ))}
          </select>
        )}
      </Field>
      <Field label={tl('court.label')} error={fieldError(error, 'court_id')}>
        {(p) => (
          <select {...p} required value={courtId} onChange={(e) => setCourtId(e.target.value)}>
            <option value="">{t('common.choose')}</option>
            {(courts ?? []).map((c) => (
              <option key={c.id} value={c.id}>
                {name(c.name)}
              </option>
            ))}
          </select>
        )}
      </Field>
      <Field label={tl('caseNumber.label')} hint={example ? t('caseNumber.example', { example }) : t('caseNumber.optional')} error={fieldError(error, 'case_number')}>
        {(p) => <input {...p} type="text" value={caseNo} onChange={(e) => setCaseNo(e.target.value)} spellCheck={false} />}
      </Field>
      <div className="grid-2">
        <Field label={tl('proceeding.titleEn')}>
          {(p) => <input {...p} type="text" lang="en" value={titleEn} onChange={(e) => setTitleEn(e.target.value)} />}
        </Field>
        <Field label={tl('proceeding.titleSi')}>
          {(p) => <input {...p} type="text" lang="si" value={titleSi} onChange={(e) => setTitleSi(e.target.value)} />}
        </Field>
        <Field label={tl('proceeding.ourSide')} error={fieldError(error, 'our_side')}>
          {(p) => (
            <select {...p} value={ourSide} onChange={(e) => setOurSide(e.target.value)}>
              <option value="">{t('common.choose')}</option>
              {OUR_SIDES.map((s) => (
                <option key={s} value={s}>
                  {t(`ourSide.${s}`)}
                </option>
              ))}
            </select>
          )}
        </Field>
        <Field label={tl('proceeding.ourRole')}>
          {(p) => (
            <select {...p} value={ourRole} onChange={(e) => setOurRole(e.target.value)}>
              {OUR_ROLES.map((s) => (
                <option key={s} value={s}>
                  {t(`ourRole.${s}`)}
                </option>
              ))}
            </select>
          )}
        </Field>
        <Field label={tl('proceeding.instituted')} error={fieldError(error, 'instituted_on')}>
          {(p) => <input {...p} type="date" value={instituted} onChange={(e) => setInstituted(e.target.value)} />}
        </Field>
      </div>
      {parties.map((p, i) => (
        <PartyFields key={i} index={i} value={p} onChange={(v) => setParties(parties.map((x, j) => (j === i ? v : x)))} />
      ))}
      <div className="row">
        <button type="button" className="btn btn--small" onClick={() => setParties([...parties, emptyParty('opposing')])}>
          {t('party.addAnother')}
        </button>
        {parties.length > 0 ? (
          <button type="button" className="btn btn--small btn--quiet" onClick={() => setParties(parties.slice(0, -1))}>
            {t('party.removeLast')}
          </button>
        ) : null}
      </div>
      <div className="row row--end">
        <button type="submit" className="btn btn--primary" disabled={busy || !typeId || !courtId}>
          {t('proceeding.save')}
        </button>
      </div>
    </form>
  );
}

function PartyForm({ proceeding, onDone }: { proceeding: Proceeding; onDone: (w?: Warning[]) => void }) {
  const { t } = useT();
  const [p, setP] = useState<PartyDraft>(emptyParty('opposing'));
  const [error, setError] = useState<unknown>(null);
  const [busy, setBusy] = useState(false);
  const submit = async (e: FormEvent) => {
    e.preventDefault();
    const body = partyPayload(p);
    if (!body) return;
    setBusy(true);
    setError(null);
    try {
      await api.post(`/api/v1/proceedings/${proceeding.id}/parties`, body);
      onDone();
    } catch (err) {
      setError(err);
    } finally {
      setBusy(false);
    }
  };
  return (
    <form className="form" onSubmit={submit} noValidate>
      <ErrorNotice error={error} />
      <PartyFields value={p} onChange={setP} index={(proceeding.parties ?? []).length} />
      <div className="row row--end">
        <button type="submit" className="btn btn--primary" disabled={busy || !partyPayload(p)}>
          {t('party.save')}
        </button>
      </div>
    </form>
  );
}

const CN_REASONS = ['assigned', 'corrected', 'renumbered', 'transferred', 'migrated'];

function CaseNumberForm({ proceeding, onDone }: { proceeding: Proceeding; onDone: (w?: Warning[]) => void }) {
  const { t, tl, name, fieldError } = useT();
  const courts = useCourts();
  const [courtId, setCourtId] = useState(proceeding.court_id);
  const [raw, setRaw] = useState('');
  const [reason, setReason] = useState(proceeding.current_case_number ? 'renumbered' : 'assigned');
  const [validFrom, setValidFrom] = useState('');
  const [error, setError] = useState<unknown>(null);
  const [busy, setBusy] = useState(false);
  const submit = async (e: FormEvent) => {
    e.preventDefault();
    setBusy(true);
    setError(null);
    try {
      const body: Record<string, unknown> = { court_id: courtId, raw: raw.trim(), reason };
      if (validFrom) body.valid_from = validFrom;
      const r = await api.post<WithWarnings<Proceeding>>(`/api/v1/proceedings/${proceeding.id}/case-numbers`, body);
      onDone(r.warnings);
    } catch (err) {
      setError(err);
    } finally {
      setBusy(false);
    }
  };
  return (
    <form className="form" onSubmit={submit} noValidate>
      <ErrorNotice error={error} />
      <Field label={tl('court.label')}>
        {(p) => (
          <select {...p} value={courtId} onChange={(e) => setCourtId(e.target.value)}>
            {(courts ?? []).map((c) => (
              <option key={c.id} value={c.id}>
                {name(c.name)}
              </option>
            ))}
          </select>
        )}
      </Field>
      <Field label={tl('caseNumber.label')} error={fieldError(error, 'raw')}>
        {(p) => <input {...p} type="text" required value={raw} onChange={(e) => setRaw(e.target.value)} spellCheck={false} />}
      </Field>
      <div className="grid-2">
        <Field label={tl('caseNumber.reasonLabel')} error={fieldError(error, 'reason')}>
          {(p) => (
            <select {...p} value={reason} onChange={(e) => setReason(e.target.value)}>
              {CN_REASONS.map((r) => (
                <option key={r} value={r}>
                  {t(`caseNumber.reason.${r}`)}
                </option>
              ))}
            </select>
          )}
        </Field>
        <Field label={tl('caseNumber.validFrom')} error={fieldError(error, 'valid_from')}>
          {(p) => <input {...p} type="date" value={validFrom} onChange={(e) => setValidFrom(e.target.value)} />}
        </Field>
      </div>
      <div className="row row--end">
        <button type="submit" className="btn btn--primary" disabled={busy || !raw.trim()}>
          {t('caseNumber.save')}
        </button>
      </div>
    </form>
  );
}

function LinkForm({ proceeding, all, onDone }: { proceeding: Proceeding; all: Proceeding[]; onDone: (w?: Warning[]) => void }) {
  const { t, tl, name, fieldError } = useT();
  const rels = useReference('relationship-types');
  const others = all.filter((p) => p.id !== proceeding.id);
  const [to, setTo] = useState('');
  const [typeId, setTypeId] = useState('');
  const [note, setNote] = useState('');
  const [error, setError] = useState<unknown>(null);
  const [busy, setBusy] = useState(false);
  const submit = async (e: FormEvent) => {
    e.preventDefault();
    setBusy(true);
    setError(null);
    try {
      const body: Record<string, unknown> = { to_proceeding_id: to, relationship_type_id: typeId };
      if (note.trim()) body.note = note.trim();
      await api.post(`/api/v1/proceedings/${proceeding.id}/relationships`, body);
      onDone();
    } catch (err) {
      setError(err);
    } finally {
      setBusy(false);
    }
  };
  if (others.length === 0) return <Notice kind="info">{t('link.noOthers')}</Notice>;
  return (
    <form className="form" onSubmit={submit} noValidate>
      <p>{t('link.explain', { caseNo: proceeding.current_case_number ?? t('hearing.noCaseNumber') })}</p>
      <ErrorNotice error={error} />
      <Field label={tl('link.type')}>
        {(p) => (
          <select {...p} required value={typeId} onChange={(e) => setTypeId(e.target.value)}>
            <option value="">{t('common.choose')}</option>
            {(rels ?? []).map((r) => (
              <option key={r.id} value={r.id}>
                {name(r.name)}
              </option>
            ))}
          </select>
        )}
      </Field>
      <Field label={tl('link.other')} error={fieldError(error, 'to_proceeding_id')}>
        {(p) => (
          <select {...p} required value={to} onChange={(e) => setTo(e.target.value)}>
            <option value="">{t('common.choose')}</option>
            {others.map((o) => (
              <option key={o.id} value={o.id}>
                {o.current_case_number ?? (name(o.title) || t('hearing.noCaseNumber'))}
              </option>
            ))}
          </select>
        )}
      </Field>
      <Field label={tl('link.note')}>
        {(p) => <input {...p} type="text" value={note} onChange={(e) => setNote(e.target.value)} />}
      </Field>
      <div className="row row--end">
        <button type="submit" className="btn btn--primary" disabled={busy || !to || !typeId}>
          {t('link.save')}
        </button>
      </div>
    </form>
  );
}

function HearingForm({ matter, proceeding, onDone }: { matter: MatterDetail; proceeding: Proceeding; onDone: (w?: Warning[]) => void }) {
  const { t, tl, name } = useT();
  const navigate = useNavigate();
  const purposes = useReference('hearing-purposes');
  const courts = useCourts();
  const key = useMemo(() => uuidv4(), []);
  const [claim, setClaim] = useState<DateClaim>(() => emptyClaim(''));
  const [purposeId, setPurposeId] = useState('');
  const [purposeNote, setPurposeNote] = useState('');
  const [courtId, setCourtId] = useState(proceeding.court_id);
  const [courtroomId, setCourtroomId] = useState('');
  const [appearing, setAppearing] = useState<string[]>([]);
  const [instructions, setInstructions] = useState('');
  const [error, setError] = useState<unknown>(null);
  const [busy, setBusy] = useState(false);
  const court = courts?.find((c) => c.id === courtId);
  const lawyers = (matter.team ?? []).filter((m) => m.team_role !== 'assistant');

  const submit = async (e: FormEvent) => {
    e.preventDefault();
    setBusy(true);
    setError(null);
    try {
      const body: Record<string, unknown> = { ...cleanClaim(claim), appearing_user_ids: appearing };
      if (purposeId) body.purpose_id = purposeId;
      if (purposeNote.trim()) body.purpose_note = purposeNote.trim();
      if (courtId && courtId !== proceeding.court_id) body.court_id = courtId;
      if (courtroomId) body.courtroom_id = courtroomId;
      if (instructions.trim()) body.instructions = instructions.trim();
      const r = await api.post<WithWarnings<HearingDetail>>(`/api/v1/proceedings/${proceeding.id}/hearings`, body, { idempotencyKey: key });
      onDone(r.warnings);
      if (r.warnings.length === 0) navigate(`/hearings/${r.data.id}`);
    } catch (err) {
      setError(err);
    } finally {
      setBusy(false);
    }
  };

  return (
    <form className="form" onSubmit={submit} noValidate>
      <ErrorNotice error={error} />
      <DateClaimFields value={claim} onChange={setClaim} error={error} />
      <div className="grid-2">
        <Field label={tl('hearing.purpose')}>
          {(p) => (
            <select {...p} value={purposeId} onChange={(e) => setPurposeId(e.target.value)}>
              <option value="">{t('common.choose')}</option>
              {(purposes ?? []).map((x) => (
                <option key={x.id} value={x.id}>
                  {name(x.name)}
                </option>
              ))}
            </select>
          )}
        </Field>
        <Field label={tl('hearing.purposeNote')}>
          {(p) => <input {...p} type="text" value={purposeNote} onChange={(e) => setPurposeNote(e.target.value)} />}
        </Field>
        <Field label={tl('court.label')}>
          {(p) => (
            <select {...p} value={courtId} onChange={(e) => setCourtId(e.target.value)}>
              {(courts ?? []).map((c) => (
                <option key={c.id} value={c.id}>
                  {name(c.name)}
                </option>
              ))}
            </select>
          )}
        </Field>
        {(court?.courtrooms ?? []).length > 0 ? (
          <Field label={tl('hearing.courtroom')}>
            {(p) => (
              <select {...p} value={courtroomId} onChange={(e) => setCourtroomId(e.target.value)}>
                <option value="">{t('common.choose')}</option>
                {(court?.courtrooms ?? []).map((r) => (
                  <option key={r.id} value={r.id}>
                    {r.label}
                  </option>
                ))}
              </select>
            )}
          </Field>
        ) : null}
      </div>
      <fieldset className="field">
        <legend>{tl('hearing.appearing')}</legend>
        <div className="choice-list">
          {lawyers.map((m) => (
            <label className="choice" key={m.user_id}>
              <input
                type="checkbox"
                checked={appearing.includes(m.user_id)}
                onChange={(e) => setAppearing(e.target.checked ? [...appearing, m.user_id] : appearing.filter((x) => x !== m.user_id))}
              />
              {m.display_name}
              <span className="muted small">{t(`teamRole.${m.team_role}`)}</span>
            </label>
          ))}
        </div>
      </fieldset>
      <Field label={tl('hearing.instructions')}>
        {(p) => <textarea {...p} rows={3} value={instructions} onChange={(e) => setInstructions(e.target.value)} />}
      </Field>
      <div className="row row--end">
        <button type="submit" className="btn btn--primary" disabled={busy || !isValidDate(claim.date) || !claim.source_type_id}>
          {t('hearingNew.save')}
        </button>
      </div>
    </form>
  );
}
