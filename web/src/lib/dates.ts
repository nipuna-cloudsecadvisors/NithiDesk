// Court dates are local calendar dates ("YYYY-MM-DD"), never instants. They
// are handled as strings and as integer y/m/d triples. A Date object is only
// used through its UTC fields, so the browser's time zone can never shift a
// court date by a day (doc 06 §2, doc 11 §6).

export const FIRM_TIME_ZONE = 'Asia/Colombo';

export interface YMD {
  y: number;
  m: number; // 1-12
  d: number;
}

const ISO_DATE = /^(\d{4})-(\d{2})-(\d{2})$/;

export function parseDate(s: string): YMD | null {
  const match = ISO_DATE.exec(s);
  if (!match) return null;
  const y = Number(match[1]);
  const m = Number(match[2]);
  const d = Number(match[3]);
  if (m < 1 || m > 12 || d < 1 || d > daysInMonth(y, m)) return null;
  return { y, m, d };
}

export function isValidDate(s: string): boolean {
  return parseDate(s) !== null;
}

function pad(n: number, width = 2): string {
  return String(n).padStart(width, '0');
}

export function formatISO({ y, m, d }: YMD): string {
  return `${pad(y, 4)}-${pad(m)}-${pad(d)}`;
}

export function daysInMonth(y: number, m: number): number {
  return new Date(Date.UTC(y, m, 0)).getUTCDate();
}

function toUTCms(s: string): number {
  const p = parseDate(s);
  if (!p) throw new RangeError(`invalid date: ${s}`);
  return Date.UTC(p.y, p.m - 1, p.d);
}

function fromUTCms(ms: number): string {
  const dt = new Date(ms);
  return formatISO({ y: dt.getUTCFullYear(), m: dt.getUTCMonth() + 1, d: dt.getUTCDate() });
}

/** Adds whole days to a court date. */
export function addDays(s: string, days: number): string {
  return fromUTCms(toUTCms(s) + days * 86_400_000);
}

/** Day of week, 0 = Sunday … 6 = Saturday. */
export function weekday(s: string): number {
  return new Date(toUTCms(s)).getUTCDay();
}

export function isWeekend(s: string): boolean {
  const w = weekday(s);
  return w === 0 || w === 6;
}

/** Difference in days, b − a. */
export function diffDays(a: string, b: string): number {
  return Math.round((toUTCms(b) - toUTCms(a)) / 86_400_000);
}

export function compareDates(a: string, b: string): number {
  return a < b ? -1 : a > b ? 1 : 0; // ISO strings sort lexically
}

/** Monday of the week containing s. */
export function startOfWeek(s: string): string {
  const w = weekday(s);
  return addDays(s, w === 0 ? -6 : 1 - w);
}

/**
 * Today's date in Asia/Colombo, whatever the device's time zone. Sri Lanka
 * has no daylight saving, so a fixed +05:30 offset is a safe fallback when
 * the runtime lacks time-zone data.
 */
export function todayInColombo(now: Date = new Date()): string {
  try {
    const parts = new Intl.DateTimeFormat('en-CA', {
      timeZone: FIRM_TIME_ZONE,
      year: 'numeric',
      month: '2-digit',
      day: '2-digit',
    }).formatToParts(now);
    const get = (type: string) => parts.find((p) => p.type === type)?.value ?? '';
    const s = `${get('year')}-${get('month')}-${get('day')}`;
    if (isValidDate(s)) return s;
  } catch {
    // fall through
  }
  return fromUTCms(now.getTime() + 330 * 60_000).slice(0, 10);
}

/** DD/MM/YYYY — the Sri Lankan short form. */
export function formatShort(s: string): string {
  const p = parseDate(s);
  if (!p) return s;
  return `${pad(p.d)}/${pad(p.m)}/${pad(p.y, 4)}`;
}

/** Parses DD/MM/YYYY (or D/M/YYYY) typed by a user into ISO. */
export function parseShort(input: string): string | null {
  const match = /^\s*(\d{1,2})[/.-](\d{1,2})[/.-](\d{4})\s*$/.exec(input);
  if (!match) return null;
  const iso = `${match[3]}-${pad(Number(match[2]))}-${pad(Number(match[1]))}`;
  return isValidDate(iso) ? iso : null;
}

export interface LongDateParts {
  weekday: number;
  day: number;
  month: number;
  year: number;
}

/** Parts for the long localized form; names come from the message catalogue. */
export function longParts(s: string): LongDateParts | null {
  const p = parseDate(s);
  if (!p) return null;
  return { weekday: weekday(s), day: p.d, month: p.m, year: p.y };
}

/** Local wall-clock time in Colombo for an instant (timestamps, not court dates). */
export function colomboDateTime(iso: string): { date: string; time: string } | null {
  const ms = Date.parse(iso);
  if (Number.isNaN(ms)) return null;
  const shifted = new Date(ms + 330 * 60_000);
  const date = fromUTCms(Date.UTC(shifted.getUTCFullYear(), shifted.getUTCMonth(), shifted.getUTCDate()));
  return { date, time: `${pad(shifted.getUTCHours())}:${pad(shifted.getUTCMinutes())}` };
}
