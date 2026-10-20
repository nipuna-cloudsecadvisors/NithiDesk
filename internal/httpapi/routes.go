package httpapi

import (
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/nipuna-cloudsecadvisors/NithiDesk/internal/briefs"
	"github.com/nipuna-cloudsecadvisors/NithiDesk/internal/courts"
	"github.com/nipuna-cloudsecadvisors/NithiDesk/internal/hearings"
	"github.com/nipuna-cloudsecadvisors/NithiDesk/internal/identity"
	"github.com/nipuna-cloudsecadvisors/NithiDesk/internal/imports"
	"github.com/nipuna-cloudsecadvisors/NithiDesk/internal/matters"
	"github.com/nipuna-cloudsecadvisors/NithiDesk/internal/platform/apperr"
	"github.com/nipuna-cloudsecadvisors/NithiDesk/internal/platform/clock"
	"github.com/nipuna-cloudsecadvisors/NithiDesk/internal/proceedings"
	"github.com/nipuna-cloudsecadvisors/NithiDesk/internal/registry"
)

// Services are the application services the API exposes.
type Services struct {
	Pool        *pgxpool.Pool
	Identity    *identity.Service
	Courts      *courts.Service
	Registry    *registry.Service
	Matters     *matters.Service
	Proceedings *proceedings.Service
	Hearings    *hearings.Service
	Imports     *imports.Service
	Briefs      *briefs.Service
}

type api struct {
	s      Services
	logger *slog.Logger
	secure bool
	// sessionMaxAge bounds the cookie; the server enforces expiry itself.
	sessionMaxAge time.Duration
}

// mountAPI registers every /api/v1 route on mux.
func mountAPI(mux *http.ServeMux, s Services, opts Options) {
	logger, secureCookies := opts.Logger, opts.SecureCookies
	a := &api{s: s, logger: logger, secure: secureCookies, sessionMaxAge: opts.SessionMaxAge}
	rate, burst := opts.AuthRatePerMinute, opts.AuthBurst
	if rate <= 0 {
		rate = 20
	}
	if burst <= 0 {
		burst = 10
	}
	authLimit := newRateLimiter(rate, burst)
	pinLimit := newRateLimiter(10, 5)
	sm := &sessionMiddleware{identity: s.Identity, logger: logger, secure: secureCookies}

	// Unauthenticated.
	mux.HandleFunc("POST /api/v1/auth/login", authLimit.limit(logger, a.login))
	mux.HandleFunc("POST /api/v1/auth/unlock", pinLimit.limit(logger, a.unlock))

	authed := func(pattern string, h http.HandlerFunc) { mux.Handle(pattern, sm.wrap(h)) }
	idem := func(pattern string, required bool, h http.HandlerFunc) {
		mux.Handle(pattern, sm.wrap(idempotent(s.Pool, logger, required, h)))
	}

	// Session and MFA.
	authed("GET /api/v1/auth/state", a.authState)
	authed("POST /api/v1/auth/mfa/totp/enroll", authLimit.limit(logger, a.enrollTOTP))
	authed("POST /api/v1/auth/mfa/totp/confirm", authLimit.limit(logger, a.confirmTOTP))
	authed("POST /api/v1/auth/mfa/verify", authLimit.limit(logger, a.verifyMFA))
	authed("POST /api/v1/auth/step-up", authLimit.limit(logger, a.stepUp))
	authed("POST /api/v1/auth/logout", a.logout)
	authed("POST /api/v1/auth/pin", a.setPIN)
	authed("POST /api/v1/auth/password", a.changePassword)
	authed("GET /api/v1/me", a.me)
	authed("PATCH /api/v1/me", a.updateMe)
	authed("GET /api/v1/me/sessions", a.listSessions)
	authed("DELETE /api/v1/me/sessions/{id}", a.revokeSession)

	// Users (directory for everyone; administration with user.manage).
	authed("GET /api/v1/users", a.listUsers)
	authed("POST /api/v1/admin/users", a.createUser)
	authed("GET /api/v1/admin/users/{id}", a.getUser)
	authed("PUT /api/v1/admin/users/{id}/roles", a.setRoles)
	authed("POST /api/v1/admin/users/{id}/disable", a.disableUser)
	authed("POST /api/v1/admin/users/{id}/enable", a.enableUser)
	authed("POST /api/v1/admin/users/{id}/reset-mfa", a.resetMFA)

	// Reference data.
	authed("GET /api/v1/reference/courts", a.listCourts)
	authed("POST /api/v1/reference/courts", a.createCourt)
	authed("GET /api/v1/reference/court-calendar", a.courtCalendar)
	authed("POST /api/v1/reference/court-calendar", a.addCalendarDay)
	authed("POST /api/v1/reference/courts/{id}/case-number-check", a.caseNumberCheck)
	authed("GET /api/v1/reference/{kind}", a.listReference)
	authed("POST /api/v1/reference/{kind}", a.createReference)
	authed("PATCH /api/v1/reference/{kind}/{id}", a.updateReference)

	// Registry.
	authed("GET /api/v1/entities", a.searchEntities)
	authed("POST /api/v1/entities", a.createEntity)

	// Matters, team, walls.
	authed("GET /api/v1/matters", a.listMatters)
	idem("POST /api/v1/matters", false, a.createMatter)
	authed("GET /api/v1/matters/{id}", a.getMatter)
	authed("PATCH /api/v1/matters/{id}", a.updateMatter)
	authed("POST /api/v1/matters/{id}/members", a.addMember)
	authed("DELETE /api/v1/matters/{id}/members/{memberId}", a.endMember)
	authed("POST /api/v1/matters/{id}/confidentiality", a.setConfidentiality)
	authed("GET /api/v1/matters/{id}/proceedings", a.listMatterProceedings)
	idem("POST /api/v1/matters/{id}/proceedings", false, a.createProceeding)
	authed("POST /api/v1/ethical-walls", a.createWall)
	authed("GET /api/v1/ethical-walls/{id}", a.getWall)
	authed("POST /api/v1/ethical-walls/{id}/removal-request", a.requestWallRemoval)
	authed("POST /api/v1/ethical-walls/{id}/removal-approval", a.approveWallRemoval)

	// Proceedings.
	authed("GET /api/v1/proceedings/{id}", a.getProceeding)
	authed("DELETE /api/v1/proceedings/{id}", a.deleteProceeding)
	authed("POST /api/v1/proceedings/{id}/case-numbers", a.addCaseNumber)
	authed("POST /api/v1/proceedings/{id}/relationships", a.addRelationship)
	authed("POST /api/v1/proceedings/{id}/parties", a.addParty)
	authed("GET /api/v1/proceedings/{id}/hearings", a.proceedingHearings)
	idem("POST /api/v1/proceedings/{id}/hearings", false, a.createHearing)
	authed("GET /api/v1/case-numbers/lookup", a.lookupCaseNumber)

	// Hearings.
	authed("GET /api/v1/hearings/{id}", a.getHearing)
	authed("PATCH /api/v1/hearings/{id}", a.updateHearing)
	idem("POST /api/v1/hearings/{id}/date-assertions", true, a.assertDate)
	authed("POST /api/v1/hearings/{id}/vacate", a.vacate)
	authed("POST /api/v1/hearings/{id}/verifications", a.verify)
	authed("POST /api/v1/hearings/{id}/verifications/{vid}/revoke", a.revokeVerification)
	authed("POST /api/v1/hearings/{id}/listing-status", a.listingStatus)
	idem("PUT /api/v1/hearings/{id}/outcome", true, a.recordOutcome)
	authed("POST /api/v1/hearings/{id}/outcome/corrections", a.correctOutcome)
	authed("POST /api/v1/hearings/{id}/appearances", a.addAppearance)
	authed("DELETE /api/v1/hearings/{id}/appearances/{userId}", a.removeAppearance)
	idem("POST /api/v1/hearings/{id}/notes", true, a.addNote)

	// Views and queues.
	authed("GET /api/v1/diary/today", a.today)
	authed("GET /api/v1/diary/week", a.week)
	authed("POST /api/v1/diary/court-list-check", a.courtListCheck)
	authed("GET /api/v1/queues/verification", a.verificationQueue)
	authed("GET /api/v1/queues/outcome-not-recorded", a.outcomeQueue)
	authed("GET /api/v1/search", a.search)

	// Daily brief.
	authed("GET /api/v1/briefs/daily", a.dailyBrief)
	authed("POST /api/v1/briefs/daily/renders", a.requestBriefPDF)
	authed("GET /api/v1/briefs/renders/{id}", a.briefRender)
	authed("GET /api/v1/briefs/renders/{id}/file", a.briefFile)

	// Imports.
	authed("GET /api/v1/imports/template.csv", a.importTemplate)
	authed("GET /api/v1/imports", a.listImports)
	authed("POST /api/v1/imports", a.uploadImport)
	authed("GET /api/v1/imports/{id}", a.getImport)
	authed("GET /api/v1/imports/{id}/rows", a.importRows)
	authed("POST /api/v1/imports/{id}/validate", a.validateImport)
	idem("POST /api/v1/imports/{id}/commit", true, a.commitImport)
	authed("POST /api/v1/imports/{id}/rollback", a.rollbackImport)

	// Anything else under /api is not found (never the SPA).
	mux.HandleFunc("/api/", func(w http.ResponseWriter, r *http.Request) { writeError(w, r, logger, apperr.NotFound) })
}

func (a *api) fail(w http.ResponseWriter, r *http.Request, err error) {
	writeError(w, r, a.logger, err)
}

func (a *api) ok(w http.ResponseWriter, v any) { writeJSON(w, http.StatusOK, v) }

// withWarnings wraps a result with non-blocking warnings.
func withWarnings[T any](data T, warnings any) map[string]any {
	return map[string]any{"data": data, "warnings": warnings}
}

func dateParam(r *http.Request, name string, fallback time.Time) (time.Time, error) {
	raw := r.URL.Query().Get(name)
	if raw == "" {
		return fallback, nil
	}
	t, err := clock.ParseDate(raw)
	if err != nil {
		return time.Time{}, apperr.Invalid(apperr.Field(name, "date.invalid"))
	}
	return t, nil
}

func intParam(r *http.Request, name string, fallback int) int {
	if v, err := strconv.Atoi(r.URL.Query().Get(name)); err == nil {
		return v
	}
	return fallback
}

// ---------------------------------------------------------------------------
// Auth
// ---------------------------------------------------------------------------

type authResponse struct {
	MFAState  string `json:"mfa_state"`
	CSRFToken string `json:"csrf_token"`
}

func (a *api) login(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Firm        string `json:"firm"`
		Email       string `json:"email"`
		Password    string `json:"password"`
		ClientKind  string `json:"client_kind"`
		DeviceLabel string `json:"device_label"`
	}
	if err := decode(r, &in); err != nil {
		a.fail(w, r, err)
		return
	}
	sess, err := a.s.Identity.Login(r.Context(), identity.LoginInput{Firm: in.Firm, Email: in.Email, Password: in.Password,
		ClientKind: in.ClientKind, DeviceLabel: in.DeviceLabel, Meta: metaOf(r)})
	if err != nil {
		a.fail(w, r, err)
		return
	}
	setSessionCookie(w, sess.NewToken, a.secure, a.sessionMaxAge)
	a.ok(w, authResponse{MFAState: sess.MFAState, CSRFToken: sess.CSRF})
}

func (a *api) authState(w http.ResponseWriter, r *http.Request) {
	sess := SessionFrom(r.Context())
	a.ok(w, authResponse{MFAState: sess.MFAState, CSRFToken: sess.CSRF})
}

func (a *api) enrollTOTP(w http.ResponseWriter, r *http.Request) {
	out, err := a.s.Identity.EnrollTOTP(r.Context(), SessionFrom(r.Context()))
	if err != nil {
		a.fail(w, r, err)
		return
	}
	a.ok(w, out)
}

func (a *api) confirmTOTP(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Code string `json:"code"`
	}
	if err := decode(r, &in); err != nil {
		a.fail(w, r, err)
		return
	}
	sess := SessionFrom(r.Context())
	codes, err := a.s.Identity.ConfirmTOTP(r.Context(), sess, in.Code)
	if err != nil {
		a.fail(w, r, err)
		return
	}
	setSessionCookie(w, sess.NewToken, a.secure, a.sessionMaxAge)
	a.ok(w, map[string]any{"mfa_state": identity.MFASatisfied, "csrf_token": sess.CSRF, "recovery_codes": codes})
}

func (a *api) verifyMFA(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Code         string `json:"code"`
		RecoveryCode string `json:"recovery_code"`
	}
	if err := decode(r, &in); err != nil {
		a.fail(w, r, err)
		return
	}
	sess := SessionFrom(r.Context())
	if err := a.s.Identity.VerifyMFA(r.Context(), sess, in.Code, in.RecoveryCode); err != nil {
		a.fail(w, r, err)
		return
	}
	if sess.NewToken != "" {
		setSessionCookie(w, sess.NewToken, a.secure, a.sessionMaxAge)
	}
	a.ok(w, authResponse{MFAState: identity.MFASatisfied, CSRFToken: sess.CSRF})
}

func (a *api) stepUp(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Password string `json:"password"`
		Code     string `json:"code"`
	}
	if err := decode(r, &in); err != nil {
		a.fail(w, r, err)
		return
	}
	if err := a.s.Identity.StepUp(r.Context(), SessionFrom(r.Context()), in.Password, in.Code); err != nil {
		a.fail(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (a *api) logout(w http.ResponseWriter, r *http.Request) {
	if err := a.s.Identity.Logout(r.Context(), SessionFrom(r.Context())); err != nil {
		a.fail(w, r, err)
		return
	}
	clearSessionCookie(w, a.secure)
	w.WriteHeader(http.StatusNoContent)
}

func (a *api) setPIN(w http.ResponseWriter, r *http.Request) {
	var in struct {
		PIN string `json:"pin"`
	}
	if err := decode(r, &in); err != nil {
		a.fail(w, r, err)
		return
	}
	if err := a.s.Identity.SetPIN(r.Context(), SessionFrom(r.Context()), in.PIN); err != nil {
		a.fail(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (a *api) unlock(w http.ResponseWriter, r *http.Request) {
	var in struct {
		PIN string `json:"pin"`
	}
	if err := decode(r, &in); err != nil {
		a.fail(w, r, err)
		return
	}
	cookie, err := r.Cookie(SessionCookie)
	if err != nil {
		a.fail(w, r, apperr.SessionRequired)
		return
	}
	if err := a.s.Identity.Unlock(r.Context(), cookie.Value, in.PIN, metaOf(r)); err != nil {
		a.fail(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (a *api) changePassword(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Current string `json:"current_password"`
		New     string `json:"new_password"`
	}
	if err := decode(r, &in); err != nil {
		a.fail(w, r, err)
		return
	}
	if err := a.s.Identity.ChangePassword(r.Context(), SessionFrom(r.Context()), in.Current, in.New); err != nil {
		a.fail(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (a *api) me(w http.ResponseWriter, r *http.Request) {
	out, err := a.s.Identity.GetMe(r.Context(), SessionFrom(r.Context()))
	if err != nil {
		a.fail(w, r, err)
		return
	}
	a.ok(w, out)
}

func (a *api) updateMe(w http.ResponseWriter, r *http.Request) {
	var in identity.MeUpdate
	if err := decode(r, &in); err != nil {
		a.fail(w, r, err)
		return
	}
	if err := a.s.Identity.UpdateMe(r.Context(), SessionFrom(r.Context()), in); err != nil {
		a.fail(w, r, err)
		return
	}
	a.me(w, r)
}

func (a *api) listSessions(w http.ResponseWriter, r *http.Request) {
	out, err := a.s.Identity.ListSessions(r.Context(), SessionFrom(r.Context()))
	if err != nil {
		a.fail(w, r, err)
		return
	}
	a.ok(w, map[string]any{"items": out})
}

func (a *api) revokeSession(w http.ResponseWriter, r *http.Request) {
	id, err := pathUUID(r, "id")
	if err == nil {
		err = a.s.Identity.RevokeSession(r.Context(), SessionFrom(r.Context()), id)
	}
	if err != nil {
		a.fail(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// ---------------------------------------------------------------------------
// Users
// ---------------------------------------------------------------------------

func (a *api) listUsers(w http.ResponseWriter, r *http.Request) {
	out, err := a.s.Identity.ListUsers(r.Context(), PrincipalFrom(r.Context()))
	if err != nil {
		a.fail(w, r, err)
		return
	}
	a.ok(w, map[string]any{"items": out})
}

func (a *api) createUser(w http.ResponseWriter, r *http.Request) {
	var in identity.CreateUserInput
	if err := decode(r, &in); err != nil {
		a.fail(w, r, err)
		return
	}
	out, err := a.s.Identity.CreateUser(r.Context(), PrincipalFrom(r.Context()), in)
	if err != nil {
		a.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, out)
}

func (a *api) getUser(w http.ResponseWriter, r *http.Request) {
	id, err := pathUUID(r, "id")
	if err != nil {
		a.fail(w, r, err)
		return
	}
	out, err := a.s.Identity.GetUser(r.Context(), PrincipalFrom(r.Context()), id)
	if err != nil {
		a.fail(w, r, err)
		return
	}
	a.ok(w, out)
}

func (a *api) setRoles(w http.ResponseWriter, r *http.Request) {
	id, err := pathUUID(r, "id")
	var in struct {
		Roles []string `json:"roles"`
	}
	if err == nil {
		err = decode(r, &in)
	}
	if err == nil {
		err = a.s.Identity.SetRoles(r.Context(), PrincipalFrom(r.Context()), id, in.Roles)
	}
	if err != nil {
		a.fail(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (a *api) setUserStatus(w http.ResponseWriter, r *http.Request, disabled bool) {
	id, err := pathUUID(r, "id")
	if err == nil {
		err = a.s.Identity.SetStatus(r.Context(), PrincipalFrom(r.Context()), id, disabled)
	}
	if err != nil {
		a.fail(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (a *api) disableUser(w http.ResponseWriter, r *http.Request) { a.setUserStatus(w, r, true) }
func (a *api) enableUser(w http.ResponseWriter, r *http.Request)  { a.setUserStatus(w, r, false) }

func (a *api) resetMFA(w http.ResponseWriter, r *http.Request) {
	id, err := pathUUID(r, "id")
	if err == nil {
		err = a.s.Identity.ResetMFA(r.Context(), PrincipalFrom(r.Context()), id)
	}
	if err != nil {
		a.fail(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// ---------------------------------------------------------------------------
// Reference data
// ---------------------------------------------------------------------------

func (a *api) listReference(w http.ResponseWriter, r *http.Request) {
	kind := kindFromPath(r.PathValue("kind"))
	out, err := a.s.Courts.List(r.Context(), PrincipalFrom(r.Context()), kind, r.URL.Query().Get("all") == "true")
	if err != nil {
		a.fail(w, r, err)
		return
	}
	a.ok(w, map[string]any{"items": out})
}

// kindFromPath maps URL kinds (court-levels) to reference kinds (court_level).
func kindFromPath(p string) string {
	switch p {
	case "court-levels":
		return courts.KindCourtLevel
	case "practice-areas":
		return courts.KindPracticeArea
	case "proceeding-types":
		return courts.KindProceedingType
	case "party-role-types":
		return courts.KindPartyRoleType
	case "hearing-purposes":
		return courts.KindHearingPurpose
	case "outcome-types":
		return courts.KindOutcomeType
	case "relationship-types":
		return courts.KindRelationshipType
	case "date-source-types":
		return courts.KindDateSourceType
	}
	return ""
}

func (a *api) createReference(w http.ResponseWriter, r *http.Request) {
	var in courts.ItemInput
	if err := decode(r, &in); err != nil {
		a.fail(w, r, err)
		return
	}
	out, err := a.s.Courts.CreateItem(r.Context(), PrincipalFrom(r.Context()), kindFromPath(r.PathValue("kind")), in)
	if err != nil {
		a.fail(w, r, err)
		return
	}
	etag(w, out.Version)
	writeJSON(w, http.StatusCreated, out)
}

func (a *api) updateReference(w http.ResponseWriter, r *http.Request) {
	id, err := pathUUID(r, "id")
	version := 0
	if err == nil {
		version, err = ifMatch(r)
	}
	var in courts.ItemInput
	if err == nil {
		err = decode(r, &in)
	}
	if err != nil {
		a.fail(w, r, err)
		return
	}
	out, err := a.s.Courts.UpdateItem(r.Context(), PrincipalFrom(r.Context()), id, version, in)
	if err != nil {
		a.fail(w, r, err)
		return
	}
	etag(w, out.Version)
	a.ok(w, out)
}

func (a *api) listCourts(w http.ResponseWriter, r *http.Request) {
	out, err := a.s.Courts.Courts(r.Context(), PrincipalFrom(r.Context()))
	if err != nil {
		a.fail(w, r, err)
		return
	}
	a.ok(w, map[string]any{"items": out})
}

func (a *api) createCourt(w http.ResponseWriter, r *http.Request) {
	var in courts.CourtInput
	if err := decode(r, &in); err != nil {
		a.fail(w, r, err)
		return
	}
	id, err := a.s.Courts.CreateCourt(r.Context(), PrincipalFrom(r.Context()), in)
	if err != nil {
		a.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"id": id})
}

func (a *api) courtCalendar(w http.ResponseWriter, r *http.Request) {
	from, err := dateParam(r, "from", clock.Today())
	to := from.AddDate(0, 3, 0)
	if err == nil {
		to, err = dateParam(r, "to", to)
	}
	if err != nil {
		a.fail(w, r, err)
		return
	}
	out, err := a.s.Courts.Calendar(r.Context(), PrincipalFrom(r.Context()), from, to)
	if err != nil {
		a.fail(w, r, err)
		return
	}
	a.ok(w, map[string]any{"items": out})
}

func (a *api) addCalendarDay(w http.ResponseWriter, r *http.Request) {
	var in courts.CalendarInput
	if err := decode(r, &in); err != nil {
		a.fail(w, r, err)
		return
	}
	if err := a.s.Courts.AddCalendarDay(r.Context(), PrincipalFrom(r.Context()), in); err != nil {
		a.fail(w, r, err)
		return
	}
	w.WriteHeader(http.StatusCreated)
}

func (a *api) caseNumberCheck(w http.ResponseWriter, r *http.Request) {
	id, err := pathUUID(r, "id")
	var in struct {
		Number string `json:"number"`
	}
	if err == nil {
		err = decode(r, &in)
	}
	if err != nil {
		a.fail(w, r, err)
		return
	}
	out, err := a.s.Courts.CheckCaseNumberForCourt(r.Context(), PrincipalFrom(r.Context()), id, in.Number)
	if err != nil {
		a.fail(w, r, err)
		return
	}
	a.ok(w, out)
}

// ---------------------------------------------------------------------------
// Registry
// ---------------------------------------------------------------------------

func (a *api) searchEntities(w http.ResponseWriter, r *http.Request) {
	out, err := a.s.Registry.Search(r.Context(), PrincipalFrom(r.Context()), r.URL.Query().Get("q"), intParam(r, "limit", 20))
	if err != nil {
		a.fail(w, r, err)
		return
	}
	a.ok(w, map[string]any{"items": out})
}

func (a *api) createEntity(w http.ResponseWriter, r *http.Request) {
	var in registry.Input
	if err := decode(r, &in); err != nil {
		a.fail(w, r, err)
		return
	}
	out, err := a.s.Registry.CreateEntity(r.Context(), PrincipalFrom(r.Context()), in)
	if err != nil {
		a.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, out)
}

// ---------------------------------------------------------------------------
// Matters
// ---------------------------------------------------------------------------

func (a *api) listMatters(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	items, next, err := a.s.Matters.List(r.Context(), PrincipalFrom(r.Context()), matters.ListFilter{
		Query: q.Get("q"), Lifecycle: q.Get("lifecycle"), Mine: q.Get("mine") == "true",
		Limit: intParam(r, "limit", 50), Cursor: q.Get("cursor")})
	if err != nil {
		a.fail(w, r, err)
		return
	}
	var cursor any
	if next != "" {
		cursor = next
	}
	a.ok(w, map[string]any{"items": items, "next_cursor": cursor})
}

func (a *api) createMatter(w http.ResponseWriter, r *http.Request) {
	var in matters.CreateInput
	if err := decode(r, &in); err != nil {
		a.fail(w, r, err)
		return
	}
	out, err := a.s.Matters.Create(r.Context(), PrincipalFrom(r.Context()), in)
	if err != nil {
		a.fail(w, r, err)
		return
	}
	etag(w, out.Version)
	writeJSON(w, http.StatusCreated, out)
}

func (a *api) getMatter(w http.ResponseWriter, r *http.Request) {
	id, err := pathUUID(r, "id")
	if err != nil {
		a.fail(w, r, err)
		return
	}
	out, err := a.s.Matters.Get(r.Context(), PrincipalFrom(r.Context()), id)
	if err != nil {
		a.fail(w, r, err)
		return
	}
	etag(w, out.Version)
	a.ok(w, out)
}

func (a *api) updateMatter(w http.ResponseWriter, r *http.Request) {
	id, err := pathUUID(r, "id")
	version := 0
	if err == nil {
		version, err = ifMatch(r)
	}
	var in matters.UpdateInput
	if err == nil {
		err = decode(r, &in)
	}
	if err != nil {
		a.fail(w, r, err)
		return
	}
	out, err := a.s.Matters.Update(r.Context(), PrincipalFrom(r.Context()), id, version, in)
	if err != nil {
		a.fail(w, r, err)
		return
	}
	etag(w, out.Version)
	a.ok(w, out)
}

func (a *api) addMember(w http.ResponseWriter, r *http.Request) {
	id, err := pathUUID(r, "id")
	var in matters.MemberInput
	if err == nil {
		err = decode(r, &in)
	}
	if err == nil {
		err = a.s.Matters.AddMember(r.Context(), PrincipalFrom(r.Context()), id, in)
	}
	if err != nil {
		a.fail(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (a *api) endMember(w http.ResponseWriter, r *http.Request) {
	id, err := pathUUID(r, "id")
	var memberID uuid.UUID
	if err == nil {
		memberID, err = pathUUID(r, "memberId")
	}
	if err == nil {
		err = a.s.Matters.EndMember(r.Context(), PrincipalFrom(r.Context()), id, memberID)
	}
	if err != nil {
		a.fail(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (a *api) setConfidentiality(w http.ResponseWriter, r *http.Request) {
	id, err := pathUUID(r, "id")
	var in struct {
		Confidentiality string `json:"confidentiality"`
	}
	if err == nil {
		err = decode(r, &in)
	}
	if err == nil {
		err = a.s.Matters.SetConfidentiality(r.Context(), PrincipalFrom(r.Context()), id, in.Confidentiality)
	}
	if err != nil {
		a.fail(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (a *api) listMatterProceedings(w http.ResponseWriter, r *http.Request) {
	id, err := pathUUID(r, "id")
	if err != nil {
		a.fail(w, r, err)
		return
	}
	out, err := a.s.Proceedings.ListForMatter(r.Context(), PrincipalFrom(r.Context()), id)
	if err != nil {
		a.fail(w, r, err)
		return
	}
	a.ok(w, map[string]any{"items": out})
}

func (a *api) createWall(w http.ResponseWriter, r *http.Request) {
	var in matters.WallInput
	if err := decode(r, &in); err != nil {
		a.fail(w, r, err)
		return
	}
	out, err := a.s.Matters.CreateWall(r.Context(), PrincipalFrom(r.Context()), in)
	if err != nil {
		a.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, out)
}

func (a *api) getWall(w http.ResponseWriter, r *http.Request) {
	id, err := pathUUID(r, "id")
	if err != nil {
		a.fail(w, r, err)
		return
	}
	out, err := a.s.Matters.GetWall(r.Context(), PrincipalFrom(r.Context()), id)
	if err != nil {
		a.fail(w, r, err)
		return
	}
	a.ok(w, out)
}

func (a *api) requestWallRemoval(w http.ResponseWriter, r *http.Request) {
	id, err := pathUUID(r, "id")
	var in struct {
		Reason string `json:"reason"`
	}
	if err == nil {
		err = decode(r, &in)
	}
	if err == nil {
		err = a.s.Matters.RequestWallRemoval(r.Context(), PrincipalFrom(r.Context()), id, in.Reason)
	}
	if err != nil {
		a.fail(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (a *api) approveWallRemoval(w http.ResponseWriter, r *http.Request) {
	id, err := pathUUID(r, "id")
	if err == nil {
		err = a.s.Matters.ApproveWallRemoval(r.Context(), PrincipalFrom(r.Context()), id)
	}
	if err != nil {
		a.fail(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// ---------------------------------------------------------------------------
// Proceedings
// ---------------------------------------------------------------------------

func (a *api) createProceeding(w http.ResponseWriter, r *http.Request) {
	id, err := pathUUID(r, "id")
	var in proceedings.CreateInput
	if err == nil {
		err = decode(r, &in)
	}
	if err != nil {
		a.fail(w, r, err)
		return
	}
	out, warnings, err := a.s.Proceedings.Create(r.Context(), PrincipalFrom(r.Context()), id, in)
	if err != nil {
		a.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, withWarnings(out, nonNilSlice(warnings)))
}

func nonNilSlice[T any](s []T) []T {
	if s == nil {
		return []T{}
	}
	return s
}

func (a *api) getProceeding(w http.ResponseWriter, r *http.Request) {
	id, err := pathUUID(r, "id")
	if err != nil {
		a.fail(w, r, err)
		return
	}
	out, err := a.s.Proceedings.Get(r.Context(), PrincipalFrom(r.Context()), id)
	if err != nil {
		a.fail(w, r, err)
		return
	}
	etag(w, out.Version)
	a.ok(w, out)
}

func (a *api) deleteProceeding(w http.ResponseWriter, r *http.Request) {
	id, err := pathUUID(r, "id")
	var in struct {
		Reason string `json:"reason"`
	}
	if err == nil {
		err = decode(r, &in)
	}
	if err == nil {
		err = a.s.Proceedings.SoftDelete(r.Context(), PrincipalFrom(r.Context()), id, in.Reason)
	}
	if err != nil {
		a.fail(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (a *api) addCaseNumber(w http.ResponseWriter, r *http.Request) {
	id, err := pathUUID(r, "id")
	var in proceedings.CaseNumberInput
	if err == nil {
		err = decode(r, &in)
	}
	if err != nil {
		a.fail(w, r, err)
		return
	}
	warnings, err := a.s.Proceedings.AddCaseNumber(r.Context(), PrincipalFrom(r.Context()), id, in)
	if err != nil {
		a.fail(w, r, err)
		return
	}
	out, err := a.s.Proceedings.Get(r.Context(), PrincipalFrom(r.Context()), id)
	if err != nil {
		a.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, withWarnings(out, nonNilSlice(warnings)))
}

func (a *api) addRelationship(w http.ResponseWriter, r *http.Request) {
	id, err := pathUUID(r, "id")
	var in proceedings.RelationshipInput
	if err == nil {
		err = decode(r, &in)
	}
	if err == nil {
		err = a.s.Proceedings.AddRelationship(r.Context(), PrincipalFrom(r.Context()), id, in)
	}
	if err != nil {
		a.fail(w, r, err)
		return
	}
	w.WriteHeader(http.StatusCreated)
}

func (a *api) addParty(w http.ResponseWriter, r *http.Request) {
	id, err := pathUUID(r, "id")
	var in proceedings.PartyInput
	if err == nil {
		err = decode(r, &in)
	}
	if err == nil {
		err = a.s.Proceedings.AddParty(r.Context(), PrincipalFrom(r.Context()), id, in)
	}
	if err != nil {
		a.fail(w, r, err)
		return
	}
	w.WriteHeader(http.StatusCreated)
}

func (a *api) lookupCaseNumber(w http.ResponseWriter, r *http.Request) {
	court, err := queryUUID(r, "court")
	if err != nil {
		a.fail(w, r, err)
		return
	}
	out, err := a.s.Proceedings.Lookup(r.Context(), PrincipalFrom(r.Context()), court, r.URL.Query().Get("number"))
	if err != nil {
		a.fail(w, r, err)
		return
	}
	a.ok(w, map[string]any{"items": out})
}

func (a *api) proceedingHearings(w http.ResponseWriter, r *http.Request) {
	id, err := pathUUID(r, "id")
	if err != nil {
		a.fail(w, r, err)
		return
	}
	if _, err := a.s.Proceedings.Get(r.Context(), PrincipalFrom(r.Context()), id); err != nil {
		a.fail(w, r, err)
		return
	}
	out, err := a.s.Hearings.ForProceeding(r.Context(), PrincipalFrom(r.Context()), id)
	if err != nil {
		a.fail(w, r, err)
		return
	}
	a.ok(w, map[string]any{"items": out})
}

// ---------------------------------------------------------------------------
// Hearings
// ---------------------------------------------------------------------------

func (a *api) hearingResult(w http.ResponseWriter, r *http.Request, status int, d *hearings.Detail, warnings []hearings.Warning, err error) {
	if err != nil {
		a.fail(w, r, err)
		return
	}
	etag(w, d.Version)
	writeJSON(w, status, withWarnings(d, nonNilSlice(warnings)))
}

func (a *api) createHearing(w http.ResponseWriter, r *http.Request) {
	id, err := pathUUID(r, "id")
	var in hearings.CreateInput
	if err == nil {
		err = decode(r, &in)
	}
	if err != nil {
		a.fail(w, r, err)
		return
	}
	d, warnings, err := a.s.Hearings.Create(r.Context(), PrincipalFrom(r.Context()), id, in)
	a.hearingResult(w, r, http.StatusCreated, d, warnings, err)
}

func (a *api) getHearing(w http.ResponseWriter, r *http.Request) {
	id, err := pathUUID(r, "id")
	if err != nil {
		a.fail(w, r, err)
		return
	}
	d, err := a.s.Hearings.Get(r.Context(), PrincipalFrom(r.Context()), id)
	if err != nil {
		a.fail(w, r, err)
		return
	}
	etag(w, d.Version)
	a.ok(w, d)
}

func (a *api) updateHearing(w http.ResponseWriter, r *http.Request) {
	id, err := pathUUID(r, "id")
	version := 0
	if err == nil {
		version, err = ifMatch(r)
	}
	var in hearings.UpdateInput
	if err == nil {
		err = decode(r, &in)
	}
	if err != nil {
		a.fail(w, r, err)
		return
	}
	d, err := a.s.Hearings.Update(r.Context(), PrincipalFrom(r.Context()), id, version, in)
	a.hearingResult(w, r, http.StatusOK, d, nil, err)
}

func (a *api) assertDate(w http.ResponseWriter, r *http.Request) {
	id, err := pathUUID(r, "id")
	var in hearings.DateClaim
	if err == nil {
		err = decode(r, &in)
	}
	if err != nil {
		a.fail(w, r, err)
		return
	}
	d, warnings, err := a.s.Hearings.AssertDate(r.Context(), PrincipalFrom(r.Context()), id, in)
	a.hearingResult(w, r, http.StatusCreated, d, warnings, err)
}

func (a *api) vacate(w http.ResponseWriter, r *http.Request) {
	id, err := pathUUID(r, "id")
	var in hearings.VacateInput
	if err == nil {
		err = decode(r, &in)
	}
	if err != nil {
		a.fail(w, r, err)
		return
	}
	d, warnings, err := a.s.Hearings.Vacate(r.Context(), PrincipalFrom(r.Context()), id, in)
	a.hearingResult(w, r, http.StatusOK, d, warnings, err)
}

func (a *api) verify(w http.ResponseWriter, r *http.Request) {
	id, err := pathUUID(r, "id")
	var in hearings.VerifyInput
	if err == nil {
		err = decode(r, &in)
	}
	if err != nil {
		a.fail(w, r, err)
		return
	}
	d, err := a.s.Hearings.Verify(r.Context(), PrincipalFrom(r.Context()), id, in)
	a.hearingResult(w, r, http.StatusCreated, d, nil, err)
}

func (a *api) revokeVerification(w http.ResponseWriter, r *http.Request) {
	id, err := pathUUID(r, "id")
	var vid uuid.UUID
	if err == nil {
		vid, err = pathUUID(r, "vid")
	}
	var in struct {
		Reason string `json:"reason"`
	}
	if err == nil {
		err = decode(r, &in)
	}
	if err != nil {
		a.fail(w, r, err)
		return
	}
	d, err := a.s.Hearings.RevokeVerification(r.Context(), PrincipalFrom(r.Context()), id, vid, in.Reason)
	a.hearingResult(w, r, http.StatusOK, d, nil, err)
}

func (a *api) listingStatus(w http.ResponseWriter, r *http.Request) {
	id, err := pathUUID(r, "id")
	var in struct {
		Status string `json:"status"`
		Reason string `json:"reason"`
	}
	if err == nil {
		err = decode(r, &in)
	}
	if err != nil {
		a.fail(w, r, err)
		return
	}
	d, err := a.s.Hearings.SetListingStatus(r.Context(), PrincipalFrom(r.Context()), id, in.Status, in.Reason)
	a.hearingResult(w, r, http.StatusOK, d, nil, err)
}

func (a *api) recordOutcome(w http.ResponseWriter, r *http.Request) {
	id, err := pathUUID(r, "id")
	var in hearings.OutcomeInput
	if err == nil {
		err = decode(r, &in)
	}
	if err != nil {
		a.fail(w, r, err)
		return
	}
	d, warnings, err := a.s.Hearings.RecordOutcome(r.Context(), PrincipalFrom(r.Context()), id, in)
	a.hearingResult(w, r, http.StatusOK, d, warnings, err)
}

func (a *api) correctOutcome(w http.ResponseWriter, r *http.Request) {
	id, err := pathUUID(r, "id")
	var in hearings.OutcomeInput
	if err == nil {
		err = decode(r, &in)
	}
	if err != nil {
		a.fail(w, r, err)
		return
	}
	d, err := a.s.Hearings.CorrectOutcome(r.Context(), PrincipalFrom(r.Context()), id, in)
	a.hearingResult(w, r, http.StatusCreated, d, nil, err)
}

func (a *api) addAppearance(w http.ResponseWriter, r *http.Request) {
	id, err := pathUUID(r, "id")
	var in struct {
		UserID uuid.UUID `json:"user_id"`
	}
	if err == nil {
		err = decode(r, &in)
	}
	if err != nil {
		a.fail(w, r, err)
		return
	}
	warnings, err := a.s.Hearings.AddAppearance(r.Context(), PrincipalFrom(r.Context()), id, in.UserID)
	if err != nil {
		a.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"warnings": nonNilSlice(warnings)})
}

func (a *api) removeAppearance(w http.ResponseWriter, r *http.Request) {
	id, err := pathUUID(r, "id")
	var userID uuid.UUID
	if err == nil {
		userID, err = pathUUID(r, "userId")
	}
	if err == nil {
		err = a.s.Hearings.RemoveAppearance(r.Context(), PrincipalFrom(r.Context()), id, userID)
	}
	if err != nil {
		a.fail(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (a *api) addNote(w http.ResponseWriter, r *http.Request) {
	id, err := pathUUID(r, "id")
	var in struct {
		Body             string     `json:"body"`
		ClientRecordedAt *time.Time `json:"client_recorded_at"`
	}
	if err == nil {
		err = decode(r, &in)
	}
	if err != nil {
		a.fail(w, r, err)
		return
	}
	noteID, err := a.s.Hearings.AddNote(r.Context(), PrincipalFrom(r.Context()), id, in.Body, in.ClientRecordedAt)
	if err != nil {
		a.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"id": noteID})
}

// ---------------------------------------------------------------------------
// Views
// ---------------------------------------------------------------------------

func (a *api) today(w http.ResponseWriter, r *http.Request) {
	date, err := dateParam(r, "date", clock.Today())
	var court *uuid.UUID
	if err == nil {
		court, err = queryUUID(r, "court")
	}
	if err != nil {
		a.fail(w, r, err)
		return
	}
	out, err := a.s.Hearings.Today(r.Context(), PrincipalFrom(r.Context()), date, r.URL.Query().Get("scope"), court)
	if err != nil {
		a.fail(w, r, err)
		return
	}
	a.ok(w, out)
}

func (a *api) week(w http.ResponseWriter, r *http.Request) {
	start, err := dateParam(r, "start", clock.Today())
	var lawyer, court *uuid.UUID
	if err == nil {
		lawyer, err = queryUUID(r, "lawyer")
	}
	if err == nil {
		court, err = queryUUID(r, "court")
	}
	if err != nil {
		a.fail(w, r, err)
		return
	}
	out, err := a.s.Hearings.Week(r.Context(), PrincipalFrom(r.Context()), start, intParam(r, "days", 7),
		r.URL.Query().Get("scope"), lawyer, court)
	if err != nil {
		a.fail(w, r, err)
		return
	}
	a.ok(w, out)
}

func (a *api) verificationQueue(w http.ResponseWriter, r *http.Request) {
	court, err := queryUUID(r, "court")
	if err != nil {
		a.fail(w, r, err)
		return
	}
	out, err := a.s.Hearings.VerificationQueue(r.Context(), PrincipalFrom(r.Context()), court, intParam(r, "within_days", 30))
	if err != nil {
		a.fail(w, r, err)
		return
	}
	a.ok(w, map[string]any{"items": out})
}

func (a *api) outcomeQueue(w http.ResponseWriter, r *http.Request) {
	out, err := a.s.Hearings.OutcomeNotRecorded(r.Context(), PrincipalFrom(r.Context()), r.URL.Query().Get("scope"))
	if err != nil {
		a.fail(w, r, err)
		return
	}
	a.ok(w, map[string]any{"items": out})
}

func (a *api) courtListCheck(w http.ResponseWriter, r *http.Request) {
	var in hearings.CourtListInput
	if err := decode(r, &in); err != nil {
		a.fail(w, r, err)
		return
	}
	out, err := a.s.Hearings.CourtListCheck(r.Context(), PrincipalFrom(r.Context()), in)
	if err != nil {
		a.fail(w, r, err)
		return
	}
	a.ok(w, out)
}

func (a *api) search(w http.ResponseWriter, r *http.Request) {
	out, err := a.s.Hearings.Search(r.Context(), PrincipalFrom(r.Context()), r.URL.Query().Get("q"))
	if err != nil {
		a.fail(w, r, err)
		return
	}
	a.ok(w, out)
}
