// Package apperr defines the error a NithiDesk service returns to its caller.
//
// The API returns codes, never sentences (docs/11-localization-plan.md, api
// outline §2.2–2.3). A code names what went wrong; the frontend's message
// catalogue renders it in Sinhala or English. An *Error never carries a
// dependency's error text, so nothing from the database or the network can
// reach a response body.
package apperr

import (
	"errors"
	"fmt"
	"net/http"
)

// Kind maps an error to an HTTP status class.
type Kind int

// Kinds, matching the status table in docs/design/api-outline.md §2.3.
const (
	KindInvalid              Kind = iota // 422 validation failed
	KindMalformed                        // 400 malformed request
	KindUnauthorized                     // 401 no or expired session, MFA or step-up required
	KindForbidden                        // 403 visible, but the role lacks the capability
	KindNotFound                         // 404 missing or not visible
	KindConflict                         // 409 business rule
	KindPrecondition                     // 412 stale If-Match
	KindPreconditionRequired             // 428 missing If-Match or Idempotency-Key
	KindRateLimited                      // 429
	KindInternal                         // 500
)

// FieldError is one invalid input field.
type FieldError struct {
	Field  string         `json:"field"`
	Code   string         `json:"code"`
	Params map[string]any `json:"params,omitempty"`
}

// Error is a coded, client-safe error.
type Error struct {
	Kind   Kind
	Code   string
	Params map[string]any
	Fields []FieldError
	// cause is kept for logs and errors.Is; it is never serialised.
	cause error
}

func (e *Error) Error() string {
	if e.cause != nil {
		return fmt.Sprintf("%s: %v", e.Code, e.cause)
	}
	return e.Code
}

// Unwrap exposes the underlying cause to errors.Is and errors.As.
func (e *Error) Unwrap() error { return e.cause }

// Status returns the HTTP status for the error.
func (e *Error) Status() int {
	switch e.Kind {
	case KindInvalid:
		return http.StatusUnprocessableEntity
	case KindMalformed:
		return http.StatusBadRequest
	case KindUnauthorized:
		return http.StatusUnauthorized
	case KindForbidden:
		return http.StatusForbidden
	case KindNotFound:
		return http.StatusNotFound
	case KindConflict:
		return http.StatusConflict
	case KindPrecondition:
		return http.StatusPreconditionFailed
	case KindPreconditionRequired:
		return http.StatusPreconditionRequired
	case KindRateLimited:
		return http.StatusTooManyRequests
	default:
		return http.StatusInternalServerError
	}
}

// WithParams returns a copy carrying parameters for the message catalogue.
func (e *Error) WithParams(params map[string]any) *Error {
	c := *e
	c.Params = params
	return &c
}

// Wrap returns a copy that records cause for logs.
func (e *Error) Wrap(cause error) *Error {
	c := *e
	c.cause = cause
	return &c
}

// New builds an error of the given kind.
func New(kind Kind, code string) *Error { return &Error{Kind: kind, Code: code} }

// Invalid builds a validation error from field errors.
func Invalid(fields ...FieldError) *Error {
	return &Error{Kind: KindInvalid, Code: "validation.failed", Fields: fields}
}

// Field is shorthand for one field error.
func Field(field, code string) FieldError { return FieldError{Field: field, Code: code} }

// NotFound is the single code for "missing" and "not visible" (I-AC-4): a
// caller can never tell the two apart.
var NotFound = New(KindNotFound, "not_found")

// Internal is returned for anything unexpected. Its cause goes to the log.
func Internal(cause error) *Error {
	return &Error{Kind: KindInternal, Code: "internal_error", cause: cause}
}

// As extracts an *Error, converting anything else to an internal error.
func As(err error) *Error {
	var appErr *Error
	if errors.As(err, &appErr) {
		return appErr
	}
	return Internal(err)
}

// Common errors shared by several modules.
var (
	SessionRequired      = New(KindUnauthorized, "auth.session_required")
	MFARequired          = New(KindUnauthorized, "auth.mfa_required")
	StepUpRequired       = New(KindUnauthorized, "auth.step_up_required")
	Forbidden            = New(KindForbidden, "forbidden.capability")
	VersionMismatch      = New(KindPrecondition, "precondition.version_mismatch")
	IfMatchRequired      = New(KindPreconditionRequired, "precondition.required").WithParams(map[string]any{"header": "If-Match"})
	IdempotencyRequired  = New(KindPreconditionRequired, "precondition.required").WithParams(map[string]any{"header": "Idempotency-Key"})
	IdempotencyKeyReused = New(KindInvalid, "idempotency.key_reused")
	Malformed            = New(KindMalformed, "request.malformed")
	RateLimited          = New(KindRateLimited, "rate_limited")
)
