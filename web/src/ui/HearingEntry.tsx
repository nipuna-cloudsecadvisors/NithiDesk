import type { ReactNode } from 'react';
import { Link } from 'react-router-dom';
import type { Card, CourtDay, I18nName } from '../api/types';
import { useT } from '../i18n/I18nProvider';
import { VerificationBadge } from './components';

/** "Perera v. Silva (+3)" — our client against the first opposing party. */
export function usePartiesLine() {
  const { t, name } = useT();
  return (clients: I18nName[] | null, opposing: I18nName[] | null): string => {
    const ours = (clients ?? []).map(name).filter(Boolean);
    const them = (opposing ?? []).map(name).filter(Boolean);
    const extra = Math.max(0, ours.length - 1) + Math.max(0, them.length - 1);
    if (ours.length === 0 && them.length === 0) return '';
    if (them.length === 0) return ours.join(', ');
    if (ours.length === 0) return t('parties.opposingOnly', { opposing: them[0] ?? '' });
    return extra > 0
      ? t('parties.versusMore', { client: ours[0] ?? '', opposing: them[0] ?? '', count: extra })
      : t('parties.versus', { client: ours[0] ?? '', opposing: them[0] ?? '' });
  };
}

function useOtherScriptParties() {
  const { otherName } = useT();
  return (clients: I18nName[] | null, opposing: I18nName[] | null) => {
    const a = (clients ?? [])[0];
    const b = (opposing ?? [])[0];
    const x = a ? otherName(a) : '';
    const y = b ? otherName(b) : '';
    return [x, y].filter(Boolean).join(' · ');
  };
}

export function sessionLabel(t: (id: string) => string, card: Pick<Card, 'session' | 'time'>): string {
  if (card.time) return card.time;
  return card.session === 'unspecified' ? '' : t(`session.${card.session}`);
}

/** A hearing in a list (Today, queues, brief). */
export function HearingEntry({ card, actions, showDate }: { card: Card; actions?: ReactNode; showDate?: boolean }) {
  const { t, tl, name, date } = useT();
  const parties = usePartiesLine();
  const other = useOtherScriptParties();
  const caseNo = card.case_number ?? t('hearing.noCaseNumber');
  const purpose = card.purpose ? name(card.purpose.name) : '';
  const purposeLine = [purpose, card.purpose_note].filter(Boolean).join(' — ');
  const when = [showDate ? date(card.date) : '', sessionLabel(t, card), card.courtroom ? t('hearing.room', { room: card.courtroom }) : '']
    .filter(Boolean)
    .join(' · ');
  const otherLine = other(card.our_clients, card.opposing);

  return (
    <li className={`entry entry--${card.verification_state}`}>
      <div className="entry__top">
        <VerificationBadge state={card.verification_state} overdue={card.verification_overdue} />
        <Link className="entry__caseno" to={`/hearings/${card.id}`}>
          {caseNo}
        </Link>
        {when ? <span className="entry__meta">{when}</span> : null}
      </div>
      {card.title_masked ? (
        <p className="entry__parties muted">{t('matter.titleMasked')}</p>
      ) : (
        <p className="entry__parties">
          {parties(card.our_clients, card.opposing) || name(card.proceeding_title) || name(card.matter_title)}
          {otherLine ? <span className="other">{otherLine}</span> : null}
        </p>
      )}
      <p className="entry__line">
        {purposeLine ? <span>{purposeLine}</span> : null}
        {purposeLine ? <span aria-hidden="true">{' · '}</span> : null}
        <span>{tl(`preparation.${card.preparation_status}`)}</span>
        {card.listing_status !== 'scheduled' ? (
          <>
            <span aria-hidden="true">{' · '}</span>
            <span>{t(`listing.${card.listing_status}`)}</span>
          </>
        ) : null}
        {card.clash ? (
          <>
            <span aria-hidden="true">{' · '}</span>
            <span className="tag tag--bad">{t('hearing.clash')}</span>
          </>
        ) : null}
        {card.appearing && card.appearing.length === 0 ? (
          <>
            <span aria-hidden="true">{' · '}</span>
            <span className="tag tag--wait">{t('hearing.noAppearingLawyer')}</span>
          </>
        ) : null}
      </p>
      {card.last_outcome ? (
        <p className="entry__line small">
          {t('hearing.lastOutcomeLine', {
            date: date(card.last_outcome.date),
            outcome: name(card.last_outcome.outcome_name),
            order: card.last_outcome.order_text ?? card.last_outcome.summary ?? '',
          })}
        </p>
      ) : null}
      {card.instructions ? (
        <p className="entry__instructions">
          <strong>{tl('hearing.instructions')}</strong>
          {': '}
          {card.instructions}
        </p>
      ) : null}
      {actions ? <div className="entry__actions">{actions}</div> : null}
    </li>
  );
}

/** Hearings grouped by court, as on Today and the daily brief. */
export function CourtGroups({ courts, actions }: { courts: CourtDay[] | null; actions?: (c: Card) => ReactNode }) {
  const { t, name, otherName } = useT();
  return (
    <>
      {(courts ?? []).map((g) => {
        // "Dates disagree" is pinned to the top of every view (doc 06 §4.2).
        const hs = [...(g.hearings ?? [])].sort(
          (a, b) => Number(b.verification_state === 'conflict') - Number(a.verification_state === 'conflict'),
        );
        return (
          <section className="court-group" key={g.court.id} aria-labelledby={`court-${g.court.id}`}>
            <div className="court-group__head">
              <h2 id={`court-${g.court.id}`}>{name(g.court.name)}</h2>
              {otherName(g.court.name) ? <span className="court-group__other">{otherName(g.court.name)}</span> : null}
              <span className="court-group__count">{t('today.hearingCount', { count: hs.length })}</span>
            </div>
            <ul className="entries">
              {hs.map((c) => (
                <HearingEntry key={c.id} card={c} actions={actions?.(c)} />
              ))}
            </ul>
          </section>
        );
      })}
    </>
  );
}
