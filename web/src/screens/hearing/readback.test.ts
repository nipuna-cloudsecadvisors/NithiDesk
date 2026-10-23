import { describe, expect, it } from 'vitest';
import { nextDateWarnings } from './RecordOutcome';

describe('read-back warnings for the next date', () => {
  const court = 'c1';
  it('flags weekends, calendar days for this court or all courts, and past dates', () => {
    const cal = [
      { id: '1', court_id: null, day: '2026-11-24', kind: 'poya', label_en: 'Il Full Moon Poya' },
      { id: '2', court_id: 'other', day: '2026-11-24', kind: 'not_sitting' },
    ];
    expect(nextDateWarnings('2026-11-24', '2026-10-20', '2026-10-20', cal, court).map((w) => w.key)).toEqual([
      'outcome.warn.calendar.poya',
    ]);
    expect(nextDateWarnings('2026-10-24', '2026-10-20', '2026-10-20', [], court).map((w) => w.key)).toEqual(['outcome.warn.weekend']);
    expect(nextDateWarnings('2026-10-19', '2026-10-15', '2026-10-20', [], court).map((w) => w.key)).toEqual(['outcome.warn.past']);
  });

  it('blocks a next date that is not after the hearing', () => {
    expect(nextDateWarnings('2026-10-20', '2026-10-20', '2026-10-20', [], court).map((w) => w.key)).toContain('outcome.warn.notAfter');
  });
});
