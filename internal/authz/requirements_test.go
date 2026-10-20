package authz_test

import (
	"strings"
	"testing"

	"github.com/nipuna-cloudsecadvisors/NithiDesk/internal/authz"
)

// The authorisation suite is a merge gate from Foundation 1 onwards. Until
// Foundation 2 implements the policy, these tests guard the gate itself: they
// fail if the list of required bypass attempts is emptied, duplicated, or
// detached from the acceptance criteria. That keeps the CI step honest instead
// of letting it pass by having nothing to run.
func TestRequiredReadPathScenariosIsWellFormed(t *testing.T) {
	scenarios := authz.RequiredReadPathScenarios
	if len(scenarios) == 0 {
		t.Fatal("the authorisation suite must declare the read paths it covers")
	}

	seen := make(map[string]struct{}, len(scenarios))
	for _, scenario := range scenarios {
		if strings.TrimSpace(scenario.ID) == "" {
			t.Error("a scenario has no id")
			continue
		}
		if _, duplicate := seen[scenario.ID]; duplicate {
			t.Errorf("scenario %q is declared twice", scenario.ID)
		}
		seen[scenario.ID] = struct{}{}

		if scenario.Criterion <= 0 {
			t.Errorf("scenario %q is not tied to an acceptance criterion", scenario.ID)
		}
		if strings.TrimSpace(scenario.Expectation) == "" {
			t.Errorf("scenario %q states no expectation", scenario.ID)
		}
	}
}

// Criterion 8 names the read paths that must be attacked by name. If one of
// them loses its scenario, the gate stops testing what the firm was promised.
func TestCriterionEightReadPathsAreCovered(t *testing.T) {
	required := []string{
		"today_view_excludes_walled_matters",
		"weekly_view_excludes_walled_matters",
		"verification_queue_excludes_walled_matters",
		"outcome_not_recorded_queue_excludes_walled_matters",
	}

	declared := make(map[string]struct{}, len(authz.RequiredReadPathScenarios))
	for _, scenario := range authz.RequiredReadPathScenarios {
		declared[scenario.ID] = struct{}{}
	}
	for _, id := range required {
		if _, ok := declared[id]; !ok {
			t.Errorf("scenario %q is required by acceptance criterion 8 but is not declared", id)
		}
	}
}
