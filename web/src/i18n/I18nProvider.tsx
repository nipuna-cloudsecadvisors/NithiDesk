import { createContext, useContext, useMemo, type ReactNode } from 'react';
import { IntlMessageFormat } from 'intl-messageformat';
import { ApiError, NetworkError, type Warning } from '../api/client';
import type { I18nName } from '../api/types';
import { formatShort, longParts } from '../lib/dates';

export type Locale = 'en' | 'si';
export type LabelMode = 'single' | 'dual';

export const LOCALES: Locale[] = ['en', 'si'];

// Catalogues are separate chunks: a user downloads only the language(s) they
// read (English is also loaded in dual-label mode). ta.json is scaffolded and
// joins LOCALES when Tamil is translated (doc 11 §11).
const catalogues: Partial<Record<Locale, Record<string, string>>> = {};
const loaders: Record<Locale, () => Promise<{ default: Record<string, string> }>> = {
  en: () => import('../locales/en.json'),
  si: () => import('../locales/si.json'),
};

export function isLoaded(l: Locale): boolean {
  return catalogues[l] !== undefined;
}

export async function loadCatalogue(l: Locale): Promise<void> {
  if (!catalogues[l]) catalogues[l] = (await loaders[l]()).default;
}

/** Everything a locale + label mode needs before it can render. */
export function cataloguesFor(locale: Locale, labelMode: LabelMode): Locale[] {
  return locale === 'si' && labelMode === 'dual' ? ['si', 'en'] : [locale];
}

type Values = Record<string, string | number | boolean | null | undefined | Date>;

export interface Translator {
  locale: Locale;
  labelMode: LabelMode;
  /** A sentence or message in the UI language. */
  t: (id: string, values?: Values) => string;
  /** A term or label: "සිංහල / English" in dual-label mode. */
  tl: (id: string, values?: Values) => string;
  has: (id: string) => boolean;
  /** Picks the user's script, falling back to the other. */
  name: (n: I18nName | null | undefined) => string;
  /** The other script's form, when it differs (for showing both). */
  otherName: (n: I18nName | null | undefined) => string;
  /** DD/MM/YYYY */
  date: (iso: string | null | undefined) => string;
  /** Tuesday 24 November 2026 / 2026 නොවැම්බර් 24, අඟහරුවාදා */
  longDate: (iso: string | null | undefined) => string;
  weekdayName: (iso: string) => string;
  errorText: (err: unknown) => string;
  fieldError: (err: unknown, field: string) => string | undefined;
  warningText: (w: Warning, names?: Record<string, string>) => string;
}

const INTL_LOCALE: Record<Locale, string> = { en: 'en-LK', si: 'si-LK' };
const compiled: Record<Locale, Map<string, IntlMessageFormat>> = { en: new Map(), si: new Map() };

/**
 * Formats one ICU message (FormatJS intl-messageformat). A missing key falls
 * back to English, then to the key itself, so a gap is visible but never fatal.
 */
export function formatMessage(locale: Locale, id: string, values?: Values): string {
  const own = catalogues[locale]?.[id];
  const loc: Locale = own !== undefined ? locale : 'en';
  const source = own ?? catalogues.en?.[id];
  if (source === undefined) return id;
  let mf = compiled[loc].get(id);
  if (!mf) {
    mf = new IntlMessageFormat(source, INTL_LOCALE[loc]);
    compiled[loc].set(id, mf);
  }
  try {
    const out = mf.format(values as Record<string, string | number | boolean | Date | null | undefined>);
    return Array.isArray(out) ? out.join('') : String(out);
  } catch {
    return source;
  }
}

export function pickName(n: I18nName | null | undefined, locale: Locale): string {
  if (!n) return '';
  const first = locale === 'si' ? n.si : n.en;
  const second = locale === 'si' ? n.en : n.si;
  return (first && first.trim()) || (second && second.trim()) || (n.ta ?? '') || '';
}

export function makeTranslator(locale: Locale, labelMode: LabelMode): Translator {
  const has = (id: string) => Object.prototype.hasOwnProperty.call(catalogues[locale] ?? {}, id);
  const t = (id: string, values?: Values) => formatMessage(locale, id, values);
  const tl = (id: string, values?: Values) => {
    const own = t(id, values);
    if (locale !== 'si' || labelMode !== 'dual') return own;
    if (!isLoaded('en')) return own;
    const english = formatMessage('en', id, values);
    return own === english ? own : `${own} / ${english}`;
  };
  const date = (iso: string | null | undefined) => (iso ? formatShort(iso) : '');
  const weekdayName = (iso: string) => {
    const p = longParts(iso);
    return p ? t(`date.weekday.${p.weekday}`) : '';
  };
  const longDate = (iso: string | null | undefined) => {
    if (!iso) return '';
    const p = longParts(iso);
    if (!p) return iso;
    return t('date.long', {
      weekday: t(`date.weekday.${p.weekday}`),
      day: String(p.day),
      month: t(`date.month.${p.month}`),
      year: String(p.year),
    });
  };
  const errorText = (err: unknown): string => {
    if (err instanceof NetworkError) return t('error.network');
    if (err instanceof ApiError) {
      const key = `error.${err.code}`;
      if (has(key)) return t(key, stringParams(err.params));
      return t('error.generic', { requestId: err.requestId || '—' });
    }
    return t('error.unexpected');
  };
  const fieldError = (err: unknown, field: string) => {
    if (!(err instanceof ApiError)) return undefined;
    const f = err.fields.find((x) => x.field === field || x.field.startsWith(`${field}.`) || x.field.startsWith(`${field}[`));
    if (!f) return undefined;
    const key = `fieldcode.${f.code}`;
    return has(key) ? t(key, stringParams({ ...err.params, ...f.params })) : t('fieldcode.invalid');
  };
  const warningText = (w: Warning, names?: Record<string, string>) => {
    const params = stringParams(w.params);
    if (w.params && typeof w.params.user_id === 'string') params.name = names?.[w.params.user_id] ?? t('common.aLawyer');
    const key = `warning.${w.code}`;
    return has(key) ? t(key, params) : t('warning.generic', { code: w.code });
  };
  return {
    locale,
    labelMode,
    t,
    tl,
    has,
    name: (n) => pickName(n, locale),
    otherName: (n) => {
      if (!n) return '';
      const main = pickName(n, locale);
      const other = locale === 'si' ? n.en : n.si;
      return other && other.trim() && other.trim() !== main ? other.trim() : '';
    },
    date,
    longDate,
    weekdayName,
    errorText,
    fieldError,
    warningText,
  };
}

function stringParams(p: Record<string, unknown> | undefined): Record<string, string> {
  const out: Record<string, string> = {};
  for (const [k, v] of Object.entries(p ?? {})) out[k] = v === null || v === undefined ? '' : String(v);
  return out;
}

const Ctx = createContext<Translator>(makeTranslator('en', 'single'));

export function I18nProvider({ locale, labelMode, children }: { locale: Locale; labelMode: LabelMode; children: ReactNode }) {
  const tr = useMemo(() => makeTranslator(locale, labelMode), [locale, labelMode]);
  return <Ctx.Provider value={tr}>{children}</Ctx.Provider>;
}

export function useT(): Translator {
  return useContext(Ctx);
}
