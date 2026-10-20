// Package observability builds the application logger.
//
// NithiDesk logs identifiers only. A client name, a case-number narrative, or
// document content must never reach the log pipeline — see
// docs/10-security-plan.md §7 and the Release 1 acceptance criteria. That is a
// hard rule, so it is enforced by the handler rather than left to reviewers:
// any attribute whose key is not on the allowed list is dropped and replaced
// with a marker naming the offending key.
package observability

import (
	"context"
	"io"
	"log/slog"
	"strings"
)

// Dropped replaces the value of an attribute that is not permitted in logs.
const Dropped = "[dropped: not permitted in logs]"

// DroppedKeysAttr collects the keys that were dropped from a record, so a
// developer can see that they tried to log something confidential.
const DroppedKeysAttr = "dropped_keys"

// allowedKeys is the complete set of attribute keys an application log line may
// carry. Everything here is an identifier, a count, a duration, or an
// operational fact. Add a key only when it cannot carry confidential content.
var allowedKeys = map[string]struct{}{
	// Record envelope.
	slog.TimeKey:    {},
	slog.LevelKey:   {},
	slog.MessageKey: {},
	slog.SourceKey:  {},

	// Who and which request.
	"actor_id":   {},
	"session_id": {},
	"device_id":  {},
	"request_id": {},
	"client_ip":  {},
	"firm_id":    {},
	"role":       {},

	// Which object, by identifier only.
	"matter_id":     {},
	"proceeding_id": {},
	"hearing_id":    {},
	"document_id":   {},
	"person_id":     {},
	"object_key":    {},
	"object_type":   {},
	"object_id":     {},

	// What happened.
	"action":   {},
	"outcome":  {},
	"reason":   {},
	"error":    {},
	"status":   {},
	"code":     {},
	"count":    {},
	"attempt":  {},
	"duration": {},

	// Operational context.
	"env":        {},
	"service":    {},
	"version":    {},
	"component":  {},
	"job":        {},
	"job_id":     {},
	"method":     {},
	"path":       {},
	"http_addr":  {},
	"bucket":     {},
	"endpoint":   {},
	"use_ssl":    {},
	"max_conns":  {},
	"interval":   {},
	"batch_size": {},
	"migration":  {},
	"signal":     {},

	// Redaction bookkeeping.
	DroppedKeysAttr: {},
}

// Options configures the logger.
type Options struct {
	Level     slog.Level
	Service   string
	Env       string
	AddSource bool
}

// New returns a JSON logger that enforces the identifiers-only rule.
func New(w io.Writer, opts Options) *slog.Logger {
	handler := slog.NewJSONHandler(w, &slog.HandlerOptions{
		Level:     opts.Level,
		AddSource: opts.AddSource,
	})
	return slog.New(&redactingHandler{inner: handler}).With(
		slog.String("service", opts.Service),
		slog.String("env", opts.Env),
	)
}

// IsAllowedKey reports whether an attribute key may be logged.
func IsAllowedKey(key string) bool {
	_, ok := allowedKeys[key]
	return ok
}

// redactingHandler drops every attribute whose key is not explicitly allowed,
// including keys nested inside groups.
type redactingHandler struct {
	inner slog.Handler
}

func (h *redactingHandler) Enabled(ctx context.Context, level slog.Level) bool {
	return h.inner.Enabled(ctx, level)
}

func (h *redactingHandler) Handle(ctx context.Context, record slog.Record) error {
	filtered := slog.NewRecord(record.Time, record.Level, record.Message, record.PC)

	var dropped []string
	record.Attrs(func(attr slog.Attr) bool {
		kept, droppedHere := filterAttr(attr)
		dropped = append(dropped, droppedHere...)
		if !kept.Equal(slog.Attr{}) {
			filtered.AddAttrs(kept)
		}
		return true
	})
	if len(dropped) > 0 {
		filtered.AddAttrs(slog.String(DroppedKeysAttr, strings.Join(dropped, ",")))
	}
	return h.inner.Handle(ctx, filtered)
}

func (h *redactingHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	kept := make([]slog.Attr, 0, len(attrs))
	for _, attr := range attrs {
		filtered, dropped := filterAttr(attr)
		if len(dropped) > 0 {
			kept = append(kept, slog.String(DroppedKeysAttr, strings.Join(dropped, ",")))
		}
		if !filtered.Equal(slog.Attr{}) {
			kept = append(kept, filtered)
		}
	}
	return &redactingHandler{inner: h.inner.WithAttrs(kept)}
}

func (h *redactingHandler) WithGroup(name string) slog.Handler {
	return &redactingHandler{inner: h.inner.WithGroup(name)}
}

// filterAttr returns the attribute to keep, plus the keys that were dropped.
// A group is filtered member by member; its own name is not a value, so a
// group name does not need to be on the allowed list.
func filterAttr(attr slog.Attr) (slog.Attr, []string) {
	value := attr.Value.Resolve()

	if value.Kind() == slog.KindGroup {
		members := value.Group()
		kept := make([]slog.Attr, 0, len(members))
		var dropped []string
		for _, member := range members {
			filtered, droppedHere := filterAttr(member)
			dropped = append(dropped, droppedHere...)
			if !filtered.Equal(slog.Attr{}) {
				kept = append(kept, filtered)
			}
		}
		if len(kept) == 0 {
			return slog.Attr{}, dropped
		}
		return slog.Attr{Key: attr.Key, Value: slog.GroupValue(kept...)}, dropped
	}

	if !IsAllowedKey(attr.Key) {
		return slog.String(attr.Key, Dropped), []string{attr.Key}
	}
	return slog.Attr{Key: attr.Key, Value: value}, nil
}
