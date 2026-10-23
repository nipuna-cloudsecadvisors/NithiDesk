package authz

// Scenario is one case the authorisation test suite must cover. The suite is a
// merge gate, so this list is the contract between Foundation 1 (which wires
// the gate) and Foundation 2 (which implements the policy and the cases).
type Scenario struct {
	// ID is a stable identifier used by test names and by review.
	ID string
	// Criterion is the Release 1 acceptance criterion it comes from, as
	// numbered in the spec document on CLO-2 section 2.
	Criterion int
	// Expectation states the behaviour that must be proven.
	Expectation string
}

// RequiredReadPathScenarios enumerates the deliberate bypass attempts the
// authorisation suite must make against every read path built in Release 1.
// Adding a read path means adding its entry here.
var RequiredReadPathScenarios = []Scenario{
	{
		ID:          "direct_url_without_matter_team",
		Criterion:   7,
		Expectation: "A user not on the matter team who types the matter URL gets not found.",
	},
	{
		ID:          "restricted_matter_returns_not_found",
		Criterion:   7,
		Expectation: "A restricted matter returns not found for an excluded user, never no access.",
	},
	{
		ID:          "restricted_matter_absent_from_search",
		Criterion:   7,
		Expectation: "A restricted matter never appears in search results for an excluded user.",
	},
	{
		ID:          "restricted_matter_absent_from_counts_and_totals",
		Criterion:   7,
		Expectation: "A restricted matter is excluded from every count, total, and aggregate an excluded user can see.",
	},
	{
		ID:          "managing_partner_is_not_implicitly_on_every_matter",
		Criterion:   6,
		Expectation: "Seniority alone grants no matter access; the managing partner sees a matter only as a member of its team.",
	},
	{
		ID:          "today_view_excludes_walled_matters",
		Criterion:   8,
		Expectation: "The Today view shows no hearing belonging to a matter the caller is walled off from.",
	},
	{
		ID:          "weekly_view_excludes_walled_matters",
		Criterion:   8,
		Expectation: "The weekly view shows no hearing belonging to a matter the caller is walled off from.",
	},
	{
		ID:          "verification_queue_excludes_walled_matters",
		Criterion:   8,
		Expectation: "The clerk verification queue excludes hearings from matters the caller is walled off from.",
	},
	{
		ID:          "outcome_not_recorded_queue_excludes_walled_matters",
		Criterion:   8,
		Expectation: "The outcome-not-recorded queue excludes hearings from matters the caller is walled off from.",
	},
	{
		ID:          "administrator_sees_no_matter_content",
		Criterion:   6,
		Expectation: "The system administrator role reaches configuration and audit, never client file content.",
	},
	{
		ID:          "audit_records_cannot_be_modified",
		Criterion:   10,
		Expectation: "No role, including the managing partner, can edit or delete an audit record through the application or the API.",
	},
}
