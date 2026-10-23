package authz

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/nipuna-cloudsecadvisors/NithiDesk/internal/platform/apperr"
	"github.com/nipuna-cloudsecadvisors/NithiDesk/internal/platform/db"
)

// Capability names a kind of action a role may ever perform (layer 1 of the
// four-layer rule, doc 09 §1). Matter access is decided separately.
type Capability string

// Capabilities used in M1 (doc 09 §6).
const (
	MatterCreate          Capability = "matter.create"
	MatterUpdate          Capability = "matter.update"
	MatterManageTeam      Capability = "matter.manage_team"
	MatterConfidentiality Capability = "matter.confidentiality"
	ProceedingWrite       Capability = "proceeding.write"
	ProceedingDelete      Capability = "proceeding.delete"
	HearingWrite          Capability = "hearing.write"
	HearingVerify         Capability = "hearing.verify"
	HearingOutcome        Capability = "hearing.outcome"
	CourtListCheck        Capability = "diary.court_list_check"
	WallManage            Capability = "wall.manage"
	ImportRun             Capability = "import.run"
	ConfigManage          Capability = "config.manage"
	UserManage            Capability = "user.manage"
	AuditReadBusiness     Capability = "audit.read_business"
	AuditReadSecurity     Capability = "audit.read_security"
	BriefPrintFirm        Capability = "brief.print_firm"
	EntityCreate          Capability = "entity.create"
)

// AllCapabilities lists every capability, for validation of role templates.
var AllCapabilities = []Capability{
	MatterCreate, MatterUpdate, MatterManageTeam, MatterConfidentiality, ProceedingWrite, ProceedingDelete,
	HearingWrite, HearingVerify, HearingOutcome, CourtListCheck, WallManage, ImportRun, ConfigManage,
	UserManage, AuditReadBusiness, AuditReadSecurity, BriefPrintFirm, EntityCreate,
}

// Matter scopes, widest last. A user with several roles gets the widest
// scope among them, except that "none" (system administrator) never widens
// another role and is never widened by one (I-AC-3).
const (
	ScopeNone         = "none"
	ScopeMember       = "member"
	ScopeDiary        = "diary"
	ScopeAllNonWalled = "all_non_walled"
)

var scopeRank = map[string]int{ScopeNone: 0, ScopeMember: 1, ScopeDiary: 2, ScopeAllNonWalled: 3}

// WidestScope combines the scopes of a user's roles.
func WidestScope(scopes []string) string {
	best := ScopeNone
	for _, s := range scopes {
		if scopeRank[s] > scopeRank[best] {
			best = s
		}
	}
	return best
}

// Principal is the authenticated caller of one request.
type Principal struct {
	FirmID       uuid.UUID
	UserID       uuid.UUID
	SessionID    uuid.UUID
	RequestID    string
	IP           string
	DisplayName  string
	Locale       string
	Roles        []string
	MatterScope  string
	Capabilities map[Capability]bool
	// StepUpFresh is true when the session re-authenticated recently enough
	// for a sensitive action (ADR 0004).
	StepUpFresh bool
}

// Can reports whether the principal's roles grant capability c.
func (p *Principal) Can(c Capability) bool { return p != nil && p.Capabilities[c] }

// Require returns forbidden.capability unless the principal has c. Use it
// only after visibility has been established: for something the caller
// cannot see, return not_found instead (I-AC-4).
func (p *Principal) Require(c Capability) error {
	if p.Can(c) {
		return nil
	}
	return apperr.Forbidden.WithParams(map[string]any{"capability": string(c)})
}

// RequireStepUp returns auth.step_up_required unless the session recently
// re-authenticated.
func (p *Principal) RequireStepUp() error {
	if p.StepUpFresh {
		return nil
	}
	return apperr.StepUpRequired
}

// Scope is the database transaction scope for this principal: row-level
// security on the firm and nd_visible_matters() on the user and scope.
func (p *Principal) Scope() db.Scope {
	return db.Scope{FirmID: p.FirmID, UserID: p.UserID, MatterScope: p.MatterScope}
}

// Access is the level at which a caller may read a matter.
type Access string

// Access levels returned by nd_visible_matters().
const (
	AccessNone  Access = ""
	AccessDiary Access = "diary" // hearing metadata only
	AccessFull  Access = "full"
)

// MatterAccess returns the caller's access to one matter, applying the same
// rule as every list query. AccessNone means the matter must be reported as
// not found.
func MatterAccess(ctx context.Context, tx pgx.Tx, matterID uuid.UUID) (Access, bool, error) {
	var access string
	var masked bool
	err := tx.QueryRow(ctx,
		`select access, title_masked from nd_visible_matters() where matter_id = $1`, matterID).Scan(&access, &masked)
	if err == pgx.ErrNoRows {
		return AccessNone, false, nil
	}
	if err != nil {
		return AccessNone, false, fmt.Errorf("matter access: %w", err)
	}
	return Access(access), masked, nil
}

// RequireMatter returns not_found unless the caller can see the matter at
// least at level min. It is the single check every matter-scoped handler
// makes before reading or writing.
func RequireMatter(ctx context.Context, tx pgx.Tx, matterID uuid.UUID, min Access) (Access, bool, error) {
	access, masked, err := MatterAccess(ctx, tx, matterID)
	if err != nil {
		return AccessNone, false, err
	}
	if access == AccessNone || (min == AccessFull && access != AccessFull) {
		return AccessNone, false, apperr.NotFound
	}
	return access, masked, nil
}

// VisibleMattersCTE is the fragment every list query starts with. Joining it
// is how a query applies the visibility rule; there is no other way to read
// matter-scoped data.
const VisibleMattersCTE = `vm as (select matter_id, access, title_masked from nd_visible_matters())`

// UserWalledFrom reports whether userID is excluded from matterID by an
// active ethical wall (I-AC-2). It is checked before adding a team member
// or an appearing lawyer.
func UserWalledFrom(ctx context.Context, tx pgx.Tx, userID, matterID uuid.UUID) (bool, error) {
	var walled bool
	err := tx.QueryRow(ctx, `
		select exists (
			select 1
			from ethical_wall_user ewu
			join ethical_wall ew on ew.firm_id = ewu.firm_id and ew.id = ewu.wall_id
			join ethical_wall_matter ewm on ewm.firm_id = ew.firm_id and ewm.wall_id = ew.id
			where ewu.user_id = $1 and ewm.matter_id = $2 and ew.removed_at is null)`,
		userID, matterID).Scan(&walled)
	if err != nil {
		return false, fmt.Errorf("check ethical wall: %w", err)
	}
	return walled, nil
}
