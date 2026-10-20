package httpapi

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/nipuna-cloudsecadvisors/NithiDesk/internal/authz"
	"github.com/nipuna-cloudsecadvisors/NithiDesk/internal/identity"
	"github.com/nipuna-cloudsecadvisors/NithiDesk/internal/platform/apperr"
	"github.com/nipuna-cloudsecadvisors/NithiDesk/internal/platform/db"
)

// Cookie names. The session cookie is HttpOnly; the CSRF token is returned in
// the body of /me and login responses and sent back in X-CSRF-Token.
const (
	SessionCookie = "nd_session"
	CSRFHeader    = "X-CSRF-Token"
	maxBodyBytes  = 1 << 20 // 1 MiB for JSON; imports have their own limit
)

type ctxKey int

const sessionKey ctxKey = 1

// SessionFrom returns the resolved session of a request.
func SessionFrom(ctx context.Context) *identity.Session {
	s, _ := ctx.Value(sessionKey).(*identity.Session)
	return s
}

// PrincipalFrom returns the caller of a request.
func PrincipalFrom(ctx context.Context) *authz.Principal {
	if s := SessionFrom(ctx); s != nil {
		return s.Principal
	}
	return nil
}

// errorBody is the error envelope (api-outline §2.3).
type errorBody struct {
	Error errorDetail `json:"error"`
}

type errorDetail struct {
	Code      string              `json:"code"`
	RequestID string              `json:"request_id"`
	Params    map[string]any      `json:"params,omitempty"`
	Fields    []apperr.FieldError `json:"fields,omitempty"`
}

// writeError renders any error as a coded response. Internal errors are
// logged with their cause; the client sees only internal_error.
func writeError(w http.ResponseWriter, r *http.Request, logger *slog.Logger, err error) {
	e := apperr.As(err)
	if e.Kind == apperr.KindInternal && logger != nil {
		logger.Error("request failed", slog.String("request_id", RequestIDFrom(r.Context())),
			slog.String("path", r.URL.Path), slog.String("error", err.Error()))
	}
	writeJSON(w, e.Status(), errorBody{Error: errorDetail{Code: e.Code, RequestID: RequestIDFrom(r.Context()),
		Params: e.Params, Fields: e.Fields}})
}

// decode reads a JSON body strictly: unknown fields are rejected so a client
// cannot set a field the handler did not mean to accept.
func decode(r *http.Request, dst any) error {
	body := http.MaxBytesReader(nil, r.Body, maxBodyBytes)
	dec := json.NewDecoder(body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		if errors.Is(err, io.EOF) {
			return apperr.Malformed
		}
		var syntax *json.SyntaxError
		var typeErr *json.UnmarshalTypeError
		switch {
		case errors.As(err, &syntax):
			return apperr.Malformed
		case errors.As(err, &typeErr):
			return apperr.Invalid(apperr.Field(typeErr.Field, "type.invalid"))
		case strings.HasPrefix(err.Error(), "json: unknown field"):
			field := strings.Trim(strings.TrimPrefix(err.Error(), "json: unknown field "), `"`)
			return apperr.Invalid(apperr.Field(field, "field.unknown"))
		}
		return apperr.Malformed
	}
	return nil
}

func pathUUID(r *http.Request, name string) (uuid.UUID, error) {
	id, err := uuid.Parse(r.PathValue(name))
	if err != nil {
		// A malformed id is reported exactly like a missing record.
		return uuid.Nil, apperr.NotFound
	}
	return id, nil
}

func queryUUID(r *http.Request, name string) (*uuid.UUID, error) {
	raw := r.URL.Query().Get(name)
	if raw == "" {
		return nil, nil
	}
	id, err := uuid.Parse(raw)
	if err != nil {
		return nil, apperr.Invalid(apperr.Field(name, "uuid.invalid"))
	}
	return &id, nil
}

// ifMatch reads the If-Match row version.
func ifMatch(r *http.Request) (int, error) {
	raw := strings.Trim(strings.TrimPrefix(r.Header.Get("If-Match"), "W/"), `"`)
	if raw == "" {
		return 0, apperr.IfMatchRequired
	}
	v, err := strconv.Atoi(raw)
	if err != nil {
		return 0, apperr.VersionMismatch
	}
	return v, nil
}

// ---------------------------------------------------------------------------
// Session middleware
// ---------------------------------------------------------------------------

// sessionMiddleware resolves the session cookie for every /api/v1 route,
// enforces MFA (only the MFA endpoints work before it is satisfied) and the
// CSRF token on every state-changing request.
type sessionMiddleware struct {
	identity *identity.Service
	logger   *slog.Logger
	secure   bool
}

// preMFA lists the routes a session may use before MFA is satisfied.
var preMFA = map[string]bool{
	"POST /api/v1/auth/mfa/totp/enroll":  true,
	"POST /api/v1/auth/mfa/totp/confirm": true,
	"POST /api/v1/auth/mfa/verify":       true,
	"POST /api/v1/auth/logout":           true,
	"GET /api/v1/auth/state":             true,
}

func (m *sessionMiddleware) wrap(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cookie, err := r.Cookie(SessionCookie)
		if err != nil {
			writeError(w, r, m.logger, apperr.SessionRequired)
			return
		}
		sess, err := m.identity.Resolve(r.Context(), cookie.Value, metaOf(r))
		if err != nil {
			if apperr.As(err).Kind == apperr.KindUnauthorized && !errors.Is(err, identity.ErrSessionLocked) {
				clearSessionCookie(w, m.secure)
			}
			writeError(w, r, m.logger, err)
			return
		}
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			if r.Header.Get(CSRFHeader) == "" || r.Header.Get(CSRFHeader) != sess.CSRF {
				writeError(w, r, m.logger, apperr.New(apperr.KindForbidden, "auth.csrf_invalid"))
				return
			}
		}
		route := r.Method + " " + r.URL.Path
		if sess.MFAState != identity.MFASatisfied && !preMFA[route] {
			writeError(w, r, m.logger, apperr.MFARequired.WithParams(map[string]any{"state": sess.MFAState}))
			return
		}
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), sessionKey, sess)))
	})
}

func metaOf(r *http.Request) identity.RequestMeta {
	return identity.RequestMeta{IP: clientIP(r), UserAgent: r.UserAgent(), RequestID: RequestIDFrom(r.Context())}
}

func setSessionCookie(w http.ResponseWriter, token string, secure bool, maxAge time.Duration) {
	http.SetCookie(w, &http.Cookie{Name: SessionCookie, Value: token, Path: "/", HttpOnly: true, Secure: secure,
		SameSite: http.SameSiteLaxMode, MaxAge: int(maxAge.Seconds())})
}

func clearSessionCookie(w http.ResponseWriter, secure bool) {
	http.SetCookie(w, &http.Cookie{Name: SessionCookie, Value: "", Path: "/", HttpOnly: true, Secure: secure,
		SameSite: http.SameSiteLaxMode, MaxAge: -1})
}

// ---------------------------------------------------------------------------
// Idempotency (api-outline §2.5)
// ---------------------------------------------------------------------------

// idempotent makes a POST/PUT safe to retry: the first response for a key is
// stored and replayed for the same key and body; the same key with another
// body is rejected. Keys are scoped to the firm and user.
func idempotent(pool *pgxpool.Pool, logger *slog.Logger, required bool, next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		key := strings.TrimSpace(r.Header.Get("Idempotency-Key"))
		if key == "" {
			if required {
				writeError(w, r, logger, apperr.IdempotencyRequired)
				return
			}
			next(w, r)
			return
		}
		if len(key) < 8 || len(key) > 200 {
			writeError(w, r, logger, apperr.Invalid(apperr.Field("Idempotency-Key", "invalid")))
			return
		}
		body, err := io.ReadAll(http.MaxBytesReader(nil, r.Body, maxBodyBytes))
		if err != nil {
			writeError(w, r, logger, apperr.Malformed)
			return
		}
		sum := sha256.Sum256(append([]byte(r.Method+" "+r.URL.Path+"\n"), body...))
		p := PrincipalFrom(r.Context())

		var storedHash []byte
		var status *int
		var stored []byte
		claimed := false
		err = db.InTx(r.Context(), pool, p.Scope(), func(tx pgx.Tx) error {
			tag, err := tx.Exec(r.Context(), `insert into idempotency_record (firm_id, user_id, idem_key, method, route,
				request_hash) values ($1, $2, $3, $4, $5, $6) on conflict do nothing`,
				p.FirmID, p.UserID, key, r.Method, r.URL.Path, sum[:])
			if err != nil {
				return err
			}
			if tag.RowsAffected() == 1 {
				claimed = true
				return nil
			}
			return tx.QueryRow(r.Context(), `select request_hash, response_status, response_body from idempotency_record
				where user_id = $1 and idem_key = $2`, p.UserID, key).Scan(&storedHash, &status, &stored)
		})
		if err != nil {
			writeError(w, r, logger, err)
			return
		}
		if !claimed {
			if string(storedHash) != string(sum[:]) {
				writeError(w, r, logger, apperr.IdempotencyKeyReused)
				return
			}
			if status == nil {
				writeError(w, r, logger, apperr.New(apperr.KindConflict, "idempotency.in_progress"))
				return
			}
			w.Header().Set("Content-Type", "application/json; charset=utf-8")
			w.Header().Set("Idempotent-Replay", "true")
			w.WriteHeader(*status)
			_, _ = w.Write(stored)
			return
		}

		r.Body = io.NopCloser(strings.NewReader(string(body)))
		rec := &captureWriter{ResponseWriter: w, status: http.StatusOK}
		next(rec, r)

		// Store successes for replay. A failure frees the key, so the client
		// can fix the request and retry with the same key.
		save := func(tx pgx.Tx) error {
			if rec.status >= 200 && rec.status < 300 {
				body := rec.buf.Bytes()
				if !json.Valid(body) {
					body = []byte("null")
				}
				_, err := tx.Exec(r.Context(), `update idempotency_record set response_status = $3, response_body = $4
					where user_id = $1 and idem_key = $2`, p.UserID, key, rec.status, body)
				return err
			}
			_, err := tx.Exec(r.Context(), `delete from idempotency_record where user_id = $1 and idem_key = $2`, p.UserID, key)
			return err
		}
		if err := db.InTx(context.WithoutCancel(r.Context()), pool, p.Scope(), save); err != nil {
			logger.Error("store idempotent response", slog.String("request_id", RequestIDFrom(r.Context())),
				slog.String("error", err.Error()))
		}
	}
}

type captureWriter struct {
	http.ResponseWriter
	status int
	buf    bytes.Buffer
}

func (c *captureWriter) WriteHeader(status int) {
	c.status = status
	c.ResponseWriter.WriteHeader(status)
}

func (c *captureWriter) Write(b []byte) (int, error) {
	c.buf.Write(b)
	return c.ResponseWriter.Write(b)
}

// ---------------------------------------------------------------------------
// Rate limiting (login, MFA, PIN): a small in-memory token bucket per IP.
// One API process per firm deployment makes this sufficient for M1; account
// lockout in the database is the durable control.
// ---------------------------------------------------------------------------

type rateLimiter struct {
	mu      sync.Mutex
	buckets map[string]*bucket
	rate    float64 // tokens per second
	burst   float64
}

type bucket struct {
	tokens float64
	last   time.Time
}

func newRateLimiter(perMinute, burst int) *rateLimiter {
	return &rateLimiter{buckets: map[string]*bucket{}, rate: float64(perMinute) / 60, burst: float64(burst)}
}

func (l *rateLimiter) allow(key string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := time.Now()
	b, ok := l.buckets[key]
	if !ok {
		if len(l.buckets) > 10000 {
			l.buckets = map[string]*bucket{} // crude bound on memory
		}
		b = &bucket{tokens: l.burst, last: now}
		l.buckets[key] = b
	}
	b.tokens = min(l.burst, b.tokens+now.Sub(b.last).Seconds()*l.rate)
	b.last = now
	if b.tokens < 1 {
		return false
	}
	b.tokens--
	return true
}

func (l *rateLimiter) limit(logger *slog.Logger, next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !l.allow(clientIP(r)) {
			w.Header().Set("Retry-After", "30")
			writeError(w, r, logger, apperr.RateLimited)
			return
		}
		next(w, r)
	}
}

// etag writes the row version as a strong ETag.
func etag(w http.ResponseWriter, version int) {
	w.Header().Set("ETag", fmt.Sprintf(`"%d"`, version))
}
