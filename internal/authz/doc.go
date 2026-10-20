// Package authz is the single place that decides authorisation.
//
// # The rule
//
// Authorisation is decided by one central policy, called inside every query:
//
//	role capability + matter access + record restriction − ethical wall
//
// A hidden button is not access control. No module may decide access for
// itself, and no query may reach the database without going through this
// package. See docs/09-role-permission-model.md and docs/10-security-plan.md.
//
// # Invisibility, not refusal
//
// A restricted matter is invisible to an excluded user — in search, lists,
// counts, totals, notifications, and reports. A read of a matter the caller
// may not see returns "not found". It never returns "no access", because
// "no access" discloses that the matter exists, which is itself a disclosure
// the ethical wall exists to prevent.
//
// # Status in Release 1
//
// Foundation 1 (this task) creates the package and wires the authorisation
// test suite into CI as a merge gate. Foundation 2 implements the policy and
// fills the suite. RequiredReadPathScenarios records what that suite has to
// cover, so the gate is a real obligation rather than an empty step: see
// requirements.go and scripts/authz-test-hook.sh.
package authz
