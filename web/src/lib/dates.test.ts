import { afterEach, describe, expect, it } from 'vitest';
import {
  addDays,
  colomboDateTime,
  diffDays,
  formatShort,
  isValidDate,
  isWeekend,
  longParts,
  parseShort,
  startOfWeek,
  todayInColombo,
  weekday,
} from './dates';

const originalTZ = process.env.TZ;
afterEach(() => {
  process.env.TZ = originalTZ;
});

describe('court dates never shift', () => {
  // Far-west and far-east zones: a Date-based implementation would move the day.
  for (const tz of ['UTC', 'Asia/Colombo', 'America/Los_Angeles', 'Pacific/Kiritimati', 'Pacific/Pago_Pago']) {
    it(`formats and computes the same in ${tz}`, () => {
      process.env.TZ = tz;
      expect(formatShort('2026-11-24')).toBe('24/11/2026');
      expect(weekday('2026-11-24')).toBe(2); // Tuesday
      expect(addDays('2026-12-31', 1)).toBe('2027-01-01');
      expect(longParts('2026-11-24')).toEqual({ weekday: 2, day: 24, month: 11, year: 2026 });
    });
  }
});

describe('date arithmetic', () => {
  it('handles month, year and leap boundaries', () => {
    expect(addDays('2028-02-28', 1)).toBe('2028-02-29');
    expect(addDays('2027-02-28', 1)).toBe('2027-03-01');
    expect(addDays('2026-03-01', -1)).toBe('2026-02-28');
    expect(addDays('2026-10-20', 35)).toBe('2026-11-24');
    expect(diffDays('2026-10-20', '2026-11-24')).toBe(35);
  });

  it('knows weekdays and weekends', () => {
    expect(weekday('2026-10-20')).toBe(2);
    expect(isWeekend('2026-10-24')).toBe(true); // Saturday
    expect(isWeekend('2026-10-25')).toBe(true); // Sunday
    expect(isWeekend('2026-10-26')).toBe(false);
    expect(startOfWeek('2026-10-25')).toBe('2026-10-19');
    expect(startOfWeek('2026-10-19')).toBe('2026-10-19');
  });

  it('validates', () => {
    expect(isValidDate('2026-02-29')).toBe(false);
    expect(isValidDate('2028-02-29')).toBe(true);
    expect(isValidDate('2026-13-01')).toBe(false);
    expect(isValidDate('24/11/2026')).toBe(false);
  });

  it('parses DD/MM/YYYY input', () => {
    expect(parseShort('24/11/2026')).toBe('2026-11-24');
    expect(parseShort('4/1/2027')).toBe('2027-01-04');
    expect(parseShort('31/02/2026')).toBeNull();
  });
});

describe('today in Asia/Colombo', () => {
  it('rolls over at midnight Colombo time, not device time', () => {
    process.env.TZ = 'America/Los_Angeles';
    // 18:29 UTC = 23:59 Colombo; 18:30 UTC = 00:00 next day in Colombo.
    expect(todayInColombo(new Date('2026-10-20T18:29:00Z'))).toBe('2026-10-20');
    expect(todayInColombo(new Date('2026-10-20T18:30:00Z'))).toBe('2026-10-21');
    // Early UTC morning is already the same day in Colombo.
    expect(todayInColombo(new Date('2026-10-20T00:10:00Z'))).toBe('2026-10-20');
  });

  it('shows timestamps in Colombo wall time', () => {
    expect(colomboDateTime('2026-10-20T19:00:00Z')).toEqual({ date: '2026-10-21', time: '00:30' });
  });
});
