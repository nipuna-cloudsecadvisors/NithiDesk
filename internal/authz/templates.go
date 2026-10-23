package authz

// RoleTemplate is a default permission bundle (doc 09 §2). Firms assign job
// titles freely and map people onto these; templates are seeded per firm and
// can then be edited, which is why they live in the database, not here.
type RoleTemplate struct {
	Code         string
	NameEN       string
	NameSI       string
	Capabilities []Capability
	MatterScope  string
}

// DefaultRoleTemplates are seeded into every new firm.
//
// Two defaults await lawyer confirmation (G0 question 8) and are therefore
// configurable per firm:
//   - The managing partner sees matters as a team member only, matching the
//     scaffold's scenario managing_partner_is_not_implicitly_on_every_matter.
//     Doc 09 allows "all non-walled matters"; a firm can switch the template.
//   - Clerks have diary scope across the firm (doc 09 §2.1).
var DefaultRoleTemplates = []RoleTemplate{
	{
		Code: "managing_partner", NameEN: "Managing Partner", NameSI: "කළමනාකාර හවුල්කරු",
		MatterScope: ScopeMember,
		Capabilities: []Capability{
			MatterCreate, MatterUpdate, MatterManageTeam, MatterConfidentiality, ProceedingWrite, ProceedingDelete,
			HearingWrite, HearingVerify, HearingOutcome, CourtListCheck, WallManage, ImportRun, AuditReadBusiness,
			BriefPrintFirm, EntityCreate,
		},
	},
	{
		Code: "partner", NameEN: "Partner", NameSI: "හවුල්කරු",
		MatterScope: ScopeMember,
		Capabilities: []Capability{
			MatterCreate, MatterUpdate, MatterManageTeam, MatterConfidentiality, ProceedingWrite, ProceedingDelete,
			HearingWrite, HearingVerify, HearingOutcome, CourtListCheck, ImportRun, EntityCreate,
		},
	},
	{
		Code: "senior_lawyer", NameEN: "Senior Lawyer", NameSI: "ජ්\u200Dයෙෂ්ඨ නීතිඥ",
		MatterScope: ScopeMember,
		Capabilities: []Capability{
			MatterCreate, MatterUpdate, ProceedingWrite, HearingWrite, HearingVerify, HearingOutcome, EntityCreate,
		},
	},
	{
		Code: "junior_lawyer", NameEN: "Junior Lawyer", NameSI: "කනිෂ්ඨ නීතිඥ",
		MatterScope:  ScopeMember,
		Capabilities: []Capability{ProceedingWrite, HearingWrite, HearingOutcome, EntityCreate},
	},
	{
		Code: "legal_assistant", NameEN: "Legal Assistant", NameSI: "නීති සහායක",
		MatterScope:  ScopeMember,
		Capabilities: []Capability{ProceedingWrite, HearingWrite, EntityCreate},
	},
	{
		Code: "court_clerk", NameEN: "Court Clerk", NameSI: "උසාවි ලිපිකරු",
		MatterScope:  ScopeDiary,
		Capabilities: []Capability{HearingVerify, CourtListCheck, BriefPrintFirm},
	},
	{
		Code: "system_administrator", NameEN: "System Administrator", NameSI: "පද්ධති පරිපාලක",
		MatterScope:  ScopeNone,
		Capabilities: []Capability{UserManage, ConfigManage, AuditReadSecurity},
	},
}
