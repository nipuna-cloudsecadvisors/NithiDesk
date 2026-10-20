import { parse, TYPE, type MessageFormatElement } from '@formatjs/icu-messageformat-parser';
import { beforeAll, describe, expect, it } from 'vitest';
import en from '../locales/en.json';
import si from '../locales/si.json';
import { ApiError, NetworkError } from '../api/client';
import { loadCatalogue, makeTranslator } from './I18nProvider';

beforeAll(async () => {
  await loadCatalogue('en');
  await loadCatalogue('si');
});

const EN = en as Record<string, string>;
const SI = si as Record<string, string>;

function argNames(msg: string): string[] {
  const out = new Set<string>();
  const walk = (els: MessageFormatElement[]) => {
    for (const e of els) {
      if (e.type === TYPE.argument || e.type === TYPE.number || e.type === TYPE.date || e.type === TYPE.time) out.add(e.value);
      if (e.type === TYPE.plural || e.type === TYPE.select) {
        out.add(e.value);
        for (const o of Object.values(e.options)) walk(o.value);
      }
      if (e.type === TYPE.tag) walk(e.children);
    }
  };
  walk(parse(msg));
  return [...out].sort();
}

describe('message catalogues', () => {
  it('en and si have exactly the same keys', () => {
    const a = Object.keys(EN).sort();
    const b = Object.keys(SI).sort();
    expect(a.filter((k) => !(k in SI))).toEqual([]);
    expect(b.filter((k) => !(k in EN))).toEqual([]);
  });

  it('every message is valid ICU and uses the same arguments in both languages', () => {
    const mismatched: string[] = [];
    for (const key of Object.keys(EN)) {
      const ea = argNames(EN[key]!);
      const sa = argNames(SI[key]!);
      if (ea.join() !== sa.join()) mismatched.push(`${key}: en(${ea}) si(${sa})`);
    }
    expect(mismatched).toEqual([]);
  });

  it('no value is empty', () => {
    expect(Object.entries(EN).filter(([, v]) => !v.trim())).toEqual([]);
    expect(Object.entries(SI).filter(([, v]) => !v.trim())).toEqual([]);
  });

  it('covers every enumerated value the API returns', () => {
    const groups: Record<string, string[]> = {
      'listing.': ['scheduled', 'held', 'not_reached', 'court_not_sitting', 'vacated', 'cancelled', 'outcome_recorded'],
      'verification.': ['verified', 'unverified', 'conflict'],
      'preparation.': ['not_started', 'in_progress', 'ready', 'blocked'],
      'session.': ['morning', 'afternoon', 'unspecified'],
      'authority.': ['authoritative', 'primary', 'secondary', 'hearsay'],
      'assertion.kind.': ['initial', 'correction', 'from_outcome', 'from_court_list', 'from_import', 'replacement'],
      'outcome.noNext.': ['concluded', 'laid_by', 'date_from_registry', 'other'],
      'role.': ['managing_partner', 'partner', 'senior_lawyer', 'junior_lawyer', 'legal_assistant', 'court_clerk', 'system_administrator'],
      'teamRole.': ['responsible_partner', 'lead', 'supporting', 'clerk', 'assistant'],
      'calendar.kind.': ['public_holiday', 'poya', 'vacation', 'not_sitting'],
      'outcome.warn.calendar.': ['public_holiday', 'poya', 'vacation', 'not_sitting'],
      'warning.hearing.date_on_': ['public_holiday', 'poya', 'vacation', 'not_sitting', 'weekend'],
      'importStatus.': ['uploaded', 'validated', 'committed', 'rolled_back', 'failed'],
      'queue.status.': ['pending', 'waiting_network', 'waiting_auth', 'needs_attention', 'expired'],
      'lifecycle.': ['open', 'on_hold', 'closure_requested', 'closed', 'archived'],
      'proceedingStatus.': ['active', 'dormant', 'concluded', 'transferred_out'],
      'partySide.': ['our_client', 'aligned', 'opposing', 'neutral'],
      'ourRole.': ['attorney_on_record', 'counsel_only', 'watching_brief'],
      'caseNumber.reason.': ['assigned', 'corrected', 'renumbered', 'transferred', 'migrated'],
      'date.weekday.': ['0', '1', '2', '3', '4', '5', '6'],
      'date.month.': ['1', '2', '3', '4', '5', '6', '7', '8', '9', '10', '11', '12'],
    };
    const missing: string[] = [];
    for (const [prefix, values] of Object.entries(groups)) {
      for (const v of values) if (!(`${prefix}${v}` in EN)) missing.push(`${prefix}${v}`);
    }
    expect(missing).toEqual([]);
  });

  it('translates every API error code the UI handles', () => {
    const codes = [
      'auth.session_required',
      'auth.session_expired',
      'auth.session_locked',
      'auth.mfa_required',
      'auth.step_up_required',
      'auth.invalid_credentials',
      'auth.account_locked',
      'auth.invalid_code',
      'auth.invalid_pin',
      'auth.csrf_invalid',
      'precondition.version_mismatch',
      'precondition.required',
      'idempotency.key_reused',
      'idempotency.in_progress',
      'validation.failed',
      'not_found',
      'internal_error',
      'rate_limited',
      'forbidden.capability',
      'hearing.outcome_already_recorded',
      'hearing.outcome_before_date',
      'hearing.outcome_requires_next_or_reason',
      'verification.assertion_not_current',
      'verification.source_not_authoritative',
    ];
    expect(codes.filter((c) => !(`error.${c}` in EN))).toEqual([]);
    const warnings = ['hearing.date_on_poya', 'hearing.appearance_clash', 'case_number.possible_duplicate', 'hearing.date_in_past'];
    expect(warnings.filter((c) => !(`warning.${c}` in EN))).toEqual([]);
  });

  it('keeps Sinhala conjuncts intact (ZWJ for yansaya / rakaransaya)', () => {
    expect(SI['date.weekday.4']).toContain('‍'); // බ්‍රහස්පතින්දා
    expect(SI['nav.outcomesPending']).toContain('‍'); // ප්‍රතිඵල
  });
});

describe('translator', () => {
  it('formats the long date in each language', () => {
    expect(makeTranslator('en', 'single').longDate('2026-11-24')).toBe('Tuesday 24 November 2026');
    expect(makeTranslator('si', 'single').longDate('2026-11-24')).toBe('2026 නොවැම්බර් 24, අඟහරුවාදා');
  });

  it('dual-label mode shows Sinhala / English for terms only', () => {
    const t = makeTranslator('si', 'dual');
    expect(t.tl('caseNumber.label')).toBe('නඩු අංකය / Case number');
    expect(t.t('caseNumber.label')).toBe('නඩු අංකය');
    expect(makeTranslator('en', 'dual').tl('caseNumber.label')).toBe('Case number');
    expect(makeTranslator('si', 'single').tl('caseNumber.label')).toBe('නඩු අංකය');
  });

  it('picks the user’s script for names with fallback', () => {
    const t = makeTranslator('si', 'single');
    expect(t.name({ en: 'Perera', si: 'පෙරේරා' })).toBe('පෙරේරා');
    expect(t.name({ en: 'Perera' })).toBe('Perera');
    expect(t.otherName({ en: 'Perera', si: 'පෙරේරා' })).toBe('Perera');
    expect(makeTranslator('en', 'single').name({ si: 'පෙරේරා' })).toBe('පෙරේරා');
  });

  it('translates error codes and falls back to a message with the request id', () => {
    const t = makeTranslator('en', 'single');
    expect(t.errorText(new ApiError(409, 'hearing.outcome_already_recorded', 'r1'))).toMatch(/already recorded/);
    expect(t.errorText(new ApiError(409, 'some.new_code', 'req-77'))).toContain('req-77');
    expect(t.errorText(new NetworkError())).toMatch(/connection/i);
    const fe = new ApiError(422, 'validation.failed', 'r', {}, [{ field: 'next_hearing.date', code: 'hearing.next_date_not_after' }]);
    expect(t.fieldError(fe, 'next_hearing')).toMatch(/after/);
  });

  it('translates warnings with parameters and resolves user names', () => {
    const t = makeTranslator('en', 'single');
    expect(t.warningText({ code: 'hearing.date_on_poya', params: { label: 'Il Full Moon Poya' } })).toContain('Il Full Moon Poya');
    expect(t.warningText({ code: 'hearing.appearance_clash', params: { user_id: 'u1' } }, { u1: 'K. Silva' })).toContain('K. Silva');
    expect(t.warningText({ code: 'brand.new' })).toContain('brand.new');
  });
});
