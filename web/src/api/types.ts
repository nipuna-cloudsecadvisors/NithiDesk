// Response and request shapes, named after the Go structs whose json tags
// they mirror (internal/hearings, matters, proceedings, courts, registry,
// identity).

export interface I18nName {
  en?: string | null;
  si?: string | null;
  ta?: string | null;
}

export type Capability =
  | 'matter.create'
  | 'matter.update'
  | 'matter.manage_team'
  | 'matter.confidentiality'
  | 'proceeding.write'
  | 'proceeding.delete'
  | 'hearing.write'
  | 'hearing.verify'
  | 'hearing.outcome'
  | 'diary.court_list_check'
  | 'wall.manage'
  | 'import.run'
  | 'config.manage'
  | 'user.manage'
  | 'audit.read_business'
  | 'audit.read_security'
  | 'brief.print_firm'
  | 'entity.create';

export interface UserView {
  id: string;
  email?: string;
  display_name: I18nName;
  status: string;
  preferred_locale: string;
  roles: string[];
  mfa_enrolled: boolean;
  last_login_at?: string;
}

export interface Me extends UserView {
  label_mode: 'single' | 'dual';
  capabilities: Capability[];
  matter_scope: string;
  client_kind: 'mobile' | 'desktop';
  csrf_token: string;
  has_pin: boolean;
  firm_name: I18nName;
  firm_time_zone: string;
}

export interface AuthResponse {
  mfa_state: 'satisfied' | 'verify_required' | 'enroll_required';
  csrf_token: string;
}

export interface TOTPEnrollment {
  otpauth_uri: string;
  secret: string;
}

export interface ConfirmResponse extends AuthResponse {
  recovery_codes: string[];
}

export interface SessionView {
  id: string;
  client_kind: string;
  device_label: string | null;
  created_at: string;
  last_seen_at: string;
  current: boolean;
}

export interface Ref {
  id: string;
  code: string;
  name: I18nName;
}

export interface Person {
  user_id: string;
  name: string;
}

export interface LastOutcome {
  hearing_id: string;
  date: string;
  outcome_code: string;
  outcome_name: I18nName;
  summary?: string;
  order_text?: string;
}

export type ListingStatus =
  | 'scheduled'
  | 'held'
  | 'not_reached'
  | 'court_not_sitting'
  | 'vacated'
  | 'cancelled'
  | 'outcome_recorded';

export type VerificationState = 'unverified' | 'verified' | 'conflict';

export type Session = 'morning' | 'afternoon' | 'unspecified';

export interface Card {
  id: string;
  date: string;
  session: Session;
  time?: string;
  court: Ref;
  courtroom?: string;
  purpose?: Ref;
  purpose_note?: string;
  listing_status: ListingStatus;
  verification_state: VerificationState;
  verification_overdue: boolean;
  preparation_status: string;
  matter_id: string;
  matter_number: string;
  matter_title: I18nName;
  title_masked?: boolean;
  proceeding_id: string;
  proceeding_title: I18nName;
  case_number?: string;
  our_clients: I18nName[] | null;
  opposing: I18nName[] | null;
  appearing: Person[] | null;
  instructions?: string;
  last_outcome?: LastOutcome;
  access: string;
  clash?: boolean;
}

export interface Assertion {
  id: string;
  date: string;
  session: Session;
  time?: string;
  source_code: string;
  source_name: I18nName;
  authority: string;
  source_detail?: string;
  kind: string;
  asserted_by?: string;
  asserted_at: string;
  current: boolean;
}

export interface VerificationView {
  id: string;
  date_assertion_id: string;
  source_code: string;
  evidence_reference: string;
  verified_by: string;
  verified_at: string;
  revoked_at?: string;
  revoke_reason?: string;
}

export interface OutcomeView {
  id: string;
  outcome_code: string;
  outcome_name: I18nName;
  summary?: string;
  order_text?: string;
  next_hearing_id?: string;
  no_next_reason?: string;
  inform_client: boolean;
  certified_copy: boolean;
  recorded_by: string;
  recorded_at: string;
  corrects_id?: string;
}

export interface CourtNote {
  id: string;
  body: string;
  created_by: string;
  created_at: string;
}

export interface HearingDetail extends Card {
  notes?: string;
  preparation_note?: string;
  status_reason?: string;
  previous_hearing_id?: string;
  replaces_hearing_id?: string;
  replaced_by_hearing_id?: string;
  next_hearing_id?: string;
  assigned_clerk?: Person;
  date_assertions: Assertion[] | null;
  verifications: VerificationView[] | null;
  outcomes: OutcomeView[] | null;
  court_notes: CourtNote[] | null;
  row_version: number;
}

export interface CalendarDay {
  id: string;
  court_id: string | null;
  day: string;
  kind: 'public_holiday' | 'poya' | 'vacation' | 'not_sitting' | string;
  label_en?: string;
  label_si?: string;
}

export interface DayCounts {
  hearings: number;
  unverified: number;
  conflicts: number;
  outcomes_not_recorded: number;
  clashes: number;
}

export interface CourtDay {
  court: Ref;
  hearings: Card[] | null;
}

export interface DayView {
  date: string;
  courts: CourtDay[] | null;
  counts: DayCounts;
  calendar_days: CalendarDay[] | null;
}

export interface WeekView {
  start: string;
  end: string;
  days: { date: string; hearings: Card[] | null }[] | null;
  highlights: {
    conflicts: number;
    verification_overdue: number;
    clashes: number;
    not_ready: number;
    no_appearing_lawyer: number;
  };
}

export interface CourtListMatch {
  number: string;
  hearing_id: string;
  verified: boolean;
}

export interface CourtListResult {
  matched: CourtListMatch[] | null;
  different_date: CourtListMatch[] | null;
  missing: Card[] | null;
  unexpected: string[] | null;
}

export interface SearchHit {
  id: string;
  matter_id: string;
  matter_number: string;
  title: I18nName;
  case_number?: string;
  court_code?: string;
  matched_on: string;
}

export interface SearchResult {
  matters: SearchHit[] | null;
  proceedings: SearchHit[] | null;
}

export interface RefItem {
  id: string;
  kind: string;
  code: string;
  name: I18nName;
  sort_order: number;
  attributes: Record<string, unknown> | null;
  valid_to?: string;
  row_version: number;
}

export interface Court {
  id: string;
  code: string;
  court_level_id: string;
  court_level_code: string;
  name: I18nName;
  location_en?: string;
  location_si?: string;
  case_number_patterns: { pattern: string; example: string }[] | null;
  sort_order: number;
  courtrooms: { id: string; label: string }[] | null;
}

export interface Entity {
  id: string;
  kind: string;
  name: I18nName;
  score?: number;
  match?: string;
}

export interface EntityInput {
  kind: string;
  name_en: string;
  name_si: string;
}

export interface MatterSummary {
  id: string;
  matter_number: string;
  title: I18nName;
  title_masked?: boolean;
  lifecycle: string;
  priority: string;
  confidentiality: string;
  opened_on: string;
  access: string;
  next_hearing_date?: string;
  next_hearing_id?: string;
  next_hearing_verification?: VerificationState;
  case_numbers: string[] | null;
}

export interface MatterDetail extends MatterSummary {
  description?: string;
  practice_area_id?: string;
  physical_file?: string;
  closed_on?: string;
  clients: { entity_id: string; name: I18nName; client_role: string }[] | null;
  team: { id: string; user_id: string; display_name: string; team_role: string; valid_from: string }[] | null;
  attention_flags: string[] | null;
  row_version: number;
}

export interface CaseNumber {
  id: string;
  court_id: string;
  raw: string;
  normalised: string;
  valid_from: string;
  valid_to?: string;
  reason: string;
  current: boolean;
}

export interface Party {
  id: string;
  entity_id: string;
  name: I18nName;
  party_role_type_id: string;
  party_role_code: string;
  ordinal?: number;
  side: string;
  status: string;
}

export interface Relationship {
  id: string;
  direction: string;
  other_proceeding_id: string;
  relationship_code: string;
  other_case_number?: string;
  note?: string;
}

export interface Proceeding {
  id: string;
  matter_id: string;
  proceeding_type_id: string;
  proceeding_type_code: string;
  court_id: string;
  title: I18nName;
  our_side?: string;
  our_role: string;
  instituted_on?: string;
  status: string;
  current_case_number?: string;
  case_numbers: CaseNumber[] | null;
  parties: Party[] | null;
  relationships: Relationship[] | null;
  access: string;
  row_version: number;
}

export interface DateClaim {
  date: string;
  session: Session;
  time?: string;
  source_type_id: string;
  source_detail?: string;
}

export interface OutcomeInput {
  outcome_type_id: string;
  summary?: string;
  order_text?: string;
  next_hearing?: {
    id?: string;
    date: string;
    session: Session;
    time?: string;
    purpose_id?: string;
    purpose_note?: string;
    source_detail?: string;
  } | null;
  no_next_reason?: string;
  no_next_note?: string;
  inform_client?: boolean;
  certified_copy?: boolean;
  appeared_user_id?: string;
  client_recorded_at?: string;
}

export interface ImportBatch {
  id: string;
  kind?: string;
  status: 'uploaded' | 'validated' | 'committed' | 'rolled_back' | 'failed' | string;
  row_count: number;
  ok_count: number;
  warning_count: number;
  error_count: number;
  row_version?: number;
  file_name?: string;
  created_at?: string;
}

export interface ImportRow {
  row_number: number;
  status: 'ok' | 'warning' | 'error';
  messages: { code: string; field?: string; params?: Record<string, unknown> }[] | null;
  raw: Record<string, string> | null;
}

export interface BriefRender {
  id: string;
  status: 'queued' | 'rendering' | 'ready' | 'failed';
  error_code?: string;
}
