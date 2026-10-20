package observability_test

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"strings"
	"testing"

	"github.com/nipuna-cloudsecadvisors/NithiDesk/internal/observability"
)

// Synthetic values only. Never put a real client name, case number, or party
// name in a test — see the repository CONTRIBUTING notes.
const (
	syntheticClientName = "Test Client Alpha"
	syntheticCaseNumber = "TEST/CASE/0001"
)

func newTestLogger(t *testing.T) (*slog.Logger, *bytes.Buffer) {
	t.Helper()
	buf := &bytes.Buffer{}
	logger := observability.New(buf, observability.Options{
		Level:   slog.LevelDebug,
		Service: "test",
		Env:     "test",
	})
	return logger, buf
}

func decode(t *testing.T, buf *bytes.Buffer) map[string]any {
	t.Helper()
	line := strings.TrimSpace(buf.String())
	if line == "" {
		t.Fatal("expected a log line, got none")
	}
	var record map[string]any
	if err := json.Unmarshal([]byte(line), &record); err != nil {
		t.Fatalf("log line is not valid JSON: %v (line %q)", err, line)
	}
	return record
}

func TestAllowedIdentifiersSurvive(t *testing.T) {
	logger, buf := newTestLogger(t)

	logger.Info("matter accessed",
		slog.String("actor_id", "user_01"),
		slog.String("matter_id", "matter_01"),
		slog.String("action", "matter.read"),
		slog.String("outcome", "success"),
	)

	record := decode(t, buf)
	for key, want := range map[string]string{
		"actor_id":  "user_01",
		"matter_id": "matter_01",
		"action":    "matter.read",
		"outcome":   "success",
		"service":   "test",
		"env":       "test",
	} {
		if got := record[key]; got != want {
			t.Errorf("record[%q] = %v, want %v", key, got, want)
		}
	}
	if _, present := record[observability.DroppedKeysAttr]; present {
		t.Errorf("no key should have been dropped, got %v", record[observability.DroppedKeysAttr])
	}
}

func TestConfidentialAttributesAreDropped(t *testing.T) {
	logger, buf := newTestLogger(t)

	logger.Info("matter accessed",
		slog.String("matter_id", "matter_01"),
		slog.String("client_name", syntheticClientName),
		slog.String("case_number", syntheticCaseNumber),
		slog.String("document_body", "privileged text"),
	)

	line := buf.String()
	for _, forbidden := range []string{syntheticClientName, syntheticCaseNumber, "privileged text"} {
		if strings.Contains(line, forbidden) {
			t.Fatalf("confidential value %q reached the log line: %s", forbidden, line)
		}
	}

	record := decode(t, buf)
	if record["matter_id"] != "matter_01" {
		t.Errorf("identifier should survive, got %v", record["matter_id"])
	}
	for _, key := range []string{"client_name", "case_number", "document_body"} {
		if record[key] != observability.Dropped {
			t.Errorf("record[%q] = %v, want the dropped marker", key, record[key])
		}
	}
	dropped, _ := record[observability.DroppedKeysAttr].(string)
	for _, key := range []string{"client_name", "case_number", "document_body"} {
		if !strings.Contains(dropped, key) {
			t.Errorf("%q should be reported in %s, got %q", key, observability.DroppedKeysAttr, dropped)
		}
	}
}

func TestConfidentialAttributesAreDroppedInsideGroups(t *testing.T) {
	logger, buf := newTestLogger(t)

	logger.Info("hearing saved",
		slog.Group("hearing",
			slog.String("hearing_id", "hearing_01"),
			slog.String("party_name", syntheticClientName),
		),
	)

	line := buf.String()
	if strings.Contains(line, syntheticClientName) {
		t.Fatalf("confidential value reached a grouped log line: %s", line)
	}

	record := decode(t, buf)
	group, ok := record["hearing"].(map[string]any)
	if !ok {
		t.Fatalf("expected a hearing group, got %T", record["hearing"])
	}
	if group["hearing_id"] != "hearing_01" {
		t.Errorf("identifier should survive inside a group, got %v", group["hearing_id"])
	}
	if group["party_name"] != observability.Dropped {
		t.Errorf("group party_name = %v, want the dropped marker", group["party_name"])
	}
}

func TestConfidentialAttributesAreDroppedFromWith(t *testing.T) {
	logger, buf := newTestLogger(t)

	logger.With(slog.String("client_name", syntheticClientName)).Info("scoped")

	if strings.Contains(buf.String(), syntheticClientName) {
		t.Fatalf("confidential value reached a With-scoped log line: %s", buf.String())
	}
}

func TestIsAllowedKey(t *testing.T) {
	tests := map[string]bool{
		"matter_id":   true,
		"actor_id":    true,
		"client_name": false,
		"case_number": false,
		"":            false,
	}
	for key, want := range tests {
		if got := observability.IsAllowedKey(key); got != want {
			t.Errorf("IsAllowedKey(%q) = %v, want %v", key, got, want)
		}
	}
}
