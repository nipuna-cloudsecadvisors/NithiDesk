// Package itest runs the API end to end against a real PostgreSQL: a fresh
// database per run, the real migrations, the real router, and users who log
// in with password + TOTP exactly as a browser would.
//
// It is the authorisation merge gate (internal/authz RequiredReadPathScenarios)
// and the diary behaviour suite. It needs NITHIDESK_TEST_DATABASE_URL, a
// connection as a role that may CREATE DATABASE and CREATE ROLE (the schema
// owner); without it the suite is skipped.
package itest

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/nipuna-cloudsecadvisors/NithiDesk/internal/audit"
	"github.com/nipuna-cloudsecadvisors/NithiDesk/internal/authz"
	"github.com/nipuna-cloudsecadvisors/NithiDesk/internal/config"
	"github.com/nipuna-cloudsecadvisors/NithiDesk/internal/courts"
	"github.com/nipuna-cloudsecadvisors/NithiDesk/internal/hearings"
	"github.com/nipuna-cloudsecadvisors/NithiDesk/internal/httpapi"
	"github.com/nipuna-cloudsecadvisors/NithiDesk/internal/identity"
	"github.com/nipuna-cloudsecadvisors/NithiDesk/internal/imports"
	"github.com/nipuna-cloudsecadvisors/NithiDesk/internal/matters"
	"github.com/nipuna-cloudsecadvisors/NithiDesk/internal/platform/migrate"
	"github.com/nipuna-cloudsecadvisors/NithiDesk/internal/platform/secretbox"
	"github.com/nipuna-cloudsecadvisors/NithiDesk/internal/proceedings"
	"github.com/nipuna-cloudsecadvisors/NithiDesk/internal/registry"
	"github.com/nipuna-cloudsecadvisors/NithiDesk/migrations"
)

const testPassword = "itest-Password-2026!"

var totpSecret = []byte("itest-totp-secret-01")

// env is one test run: a database, the API server and a firm bootstrap.
type env struct {
	t      *testing.T
	owner  *pgx.Conn
	pool   *pgxpool.Pool
	server *httptest.Server
	box    *secretbox.Box
	firms  map[string]uuid.UUID
	users  map[string]uuid.UUID // email -> id
}

func newEnv(t *testing.T) *env {
	t.Helper()
	adminURL := os.Getenv("NITHIDESK_TEST_DATABASE_URL")
	if adminURL == "" {
		t.Skip("NITHIDESK_TEST_DATABASE_URL not set; skipping integration tests")
	}
	ctx := context.Background()
	admin, err := pgx.Connect(ctx, adminURL)
	if err != nil {
		t.Fatalf("connect admin: %v", err)
	}
	name := "nithidesk_itest_" + randHex(6)
	if _, err := admin.Exec(ctx, "create database "+name); err != nil {
		t.Fatalf("create database: %v", err)
	}
	t.Cleanup(func() {
		_, _ = admin.Exec(context.Background(), "drop database if exists "+name+" with (force)")
		_ = admin.Close(context.Background())
	})

	ownerURL := withDatabase(t, adminURL, name)
	owner, err := pgx.Connect(ctx, ownerURL)
	if err != nil {
		t.Fatalf("connect owner: %v", err)
	}
	t.Cleanup(func() { _ = owner.Close(context.Background()) })
	ms, err := migrate.Load(migrations.FS)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := migrate.Up(ctx, owner, ms); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	// The API connects as a member of nithidesk_app so RLS applies.
	appPassword := "itest-" + randHex(12)
	if _, err := owner.Exec(ctx, `do $$ begin
		if not exists (select 1 from pg_roles where rolname = 'nithidesk_itest') then
			create role nithidesk_itest login in role nithidesk_app;
		end if; end $$`); err != nil {
		t.Fatalf("create app role: %v", err)
	}
	if _, err := owner.Exec(ctx, "alter role nithidesk_itest login password '"+appPassword+"'"); err != nil {
		t.Fatal(err)
	}
	if _, err := owner.Exec(ctx, "grant nithidesk_app to nithidesk_itest"); err != nil {
		t.Fatal(err)
	}
	appURL := withCredentials(t, ownerURL, "nithidesk_itest", appPassword)
	pool, err := pgxpool.New(ctx, appURL)
	if err != nil {
		t.Fatalf("app pool: %v", err)
	}
	t.Cleanup(pool.Close)

	box, err := secretbox.New(bytes.Repeat([]byte{7}, 32))
	if err != nil {
		t.Fatal(err)
	}
	sessionCfg := config.Session{CookieSecure: false, IdleDesktop: 30 * time.Minute, IdleMobile: 15 * time.Minute,
		Absolute: 12 * time.Hour, StepUpValidity: 5 * time.Minute}
	hearingSvc := hearings.New(pool)
	services := httpapi.Services{
		Pool: pool, Identity: identity.New(pool, box, sessionCfg, "alpha"), Courts: courts.New(pool),
		Registry: registry.New(pool), Matters: matters.New(pool), Proceedings: proceedings.New(pool),
		Hearings: hearingSvc, Imports: imports.New(pool),
	}
	router := httpapi.NewRouter(httpapi.Options{
		Logger: slog.New(slog.NewJSONHandler(io.Discard, nil)), Version: "itest", API: &services,
		SessionMaxAge: 12 * time.Hour, AuthRatePerMinute: 6000, AuthBurst: 1000,
	})
	server := httptest.NewServer(router)
	t.Cleanup(server.Close)

	e := &env{t: t, owner: owner, pool: pool, server: server, box: box, firms: map[string]uuid.UUID{},
		users: map[string]uuid.UUID{}}
	return e
}

func randHex(n int) string {
	b := make([]byte, n)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

func withDatabase(t *testing.T, raw, db string) string {
	u, err := url.Parse(raw)
	if err != nil {
		t.Fatal(err)
	}
	u.Path = "/" + db
	return u.String()
}

func withCredentials(t *testing.T, raw, user, password string) string {
	u, err := url.Parse(raw)
	if err != nil {
		t.Fatal(err)
	}
	u.User = url.UserPassword(user, password)
	return u.String()
}

// firm creates a firm with the default role templates and reference data.
func (e *env) firm(code string) uuid.UUID {
	e.t.Helper()
	ctx := context.Background()
	var id uuid.UUID
	err := pgx.BeginFunc(ctx, e.owner, func(tx pgx.Tx) error {
		if err := tx.QueryRow(ctx, `insert into firm (code, name_en) values ($1, $1) returning id`, code).Scan(&id); err != nil {
			return err
		}
		for _, rt := range authz.DefaultRoleTemplates {
			caps := make([]string, len(rt.Capabilities))
			for i, c := range rt.Capabilities {
				caps[i] = string(c)
			}
			if _, err := tx.Exec(ctx, `insert into role_template (firm_id, code, name_en, capabilities, matter_scope)
				values ($1, $2, $3, $4, $5)`, id, rt.Code, rt.NameEN, caps, rt.MatterScope); err != nil {
				return err
			}
		}
		return courts.SeedFirm(ctx, tx, id)
	})
	if err != nil {
		e.t.Fatalf("create firm: %v", err)
	}
	e.firms[code] = id
	return id
}

// user creates an active user with a confirmed TOTP factor.
func (e *env) user(firm, email string, roles ...string) uuid.UUID {
	e.t.Helper()
	ctx := context.Background()
	hash, err := identity.HashPassword(testPassword)
	if err != nil {
		e.t.Fatal(err)
	}
	var id uuid.UUID
	firmID := e.firms[firm]
	err = pgx.BeginFunc(ctx, e.owner, func(tx pgx.Tx) error {
		if err := tx.QueryRow(ctx, `insert into app_user (firm_id, email, display_name_en, status, password_hash)
			values ($1, $2, $2, 'active', $3) returning id`, firmID, email, hash).Scan(&id); err != nil {
			return err
		}
		for _, r := range roles {
			if _, err := tx.Exec(ctx, `insert into user_role (firm_id, user_id, role_template_id)
				select $1, $2, id from role_template where firm_id = $1 and code = $3`, firmID, id, r); err != nil {
				return err
			}
		}
		sealed, err := e.box.Seal(totpSecret, []byte("totp:"+id.String()))
		if err != nil {
			return err
		}
		_, err = tx.Exec(ctx, `insert into mfa_factor (firm_id, user_id, kind, secret_encrypted, confirmed_at)
			values ($1, $2, 'totp', $3, now())`, firmID, id, sealed)
		return err
	})
	if err != nil {
		e.t.Fatalf("create user: %v", err)
	}
	e.users[email] = id
	return id
}

// client is a logged-in browser.
type client struct {
	e     *env
	http  *http.Client
	csrf  string
	email string
}

func (e *env) login(firm, email string) *client {
	e.t.Helper()
	jar, _ := cookiejar.New(nil)
	c := &client{e: e, http: &http.Client{Jar: jar}, email: email}
	var out struct {
		MFAState  string `json:"mfa_state"`
		CSRFToken string `json:"csrf_token"`
	}
	if st := c.do("POST", "/api/v1/auth/login", map[string]any{"firm": firm, "email": email, "password": testPassword}, &out); st != 200 {
		e.t.Fatalf("login %s: status %d", email, st)
	}
	c.csrf = out.CSRFToken
	if out.MFAState != identity.MFAVerifyRequired {
		e.t.Fatalf("login %s: mfa state %q", email, out.MFAState)
	}
	code := identity.TOTPCode(totpSecret, time.Now())
	if st := c.do("POST", "/api/v1/auth/mfa/verify", map[string]any{"code": code}, &out); st != 200 {
		// The same code may already have been used by another user in this
		// second; codes are per user, so this only fails for a real error.
		e.t.Fatalf("mfa %s: status %d", email, st)
	}
	return c
}

// do sends a JSON request and decodes the response into out (if non-nil).
func (c *client) do(method, path string, body any, out any, headers ...string) int {
	c.e.t.Helper()
	var rdr io.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		rdr = bytes.NewReader(b)
	}
	req, _ := http.NewRequest(method, c.e.server.URL+path, rdr)
	req.Header.Set("Content-Type", "application/json")
	if c.csrf != "" {
		req.Header.Set(httpapi.CSRFHeader, c.csrf)
	}
	for i := 0; i+1 < len(headers); i += 2 {
		req.Header.Set(headers[i], headers[i+1])
	}
	resp, err := c.http.Do(req)
	if err != nil {
		c.e.t.Fatalf("%s %s: %v", method, path, err)
	}
	defer func() { _ = resp.Body.Close() }()
	raw, _ := io.ReadAll(resp.Body)
	if out != nil && len(raw) > 0 {
		if err := json.Unmarshal(raw, out); err != nil {
			c.e.t.Fatalf("%s %s: decode %q: %v", method, path, string(raw), err)
		}
	}
	return resp.StatusCode
}

// code extracts the error code of a failed response.
func (c *client) errCode(method, path string, body any, headers ...string) (int, string) {
	var out struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	st := c.do(method, path, body, &out, headers...)
	return st, out.Error.Code
}

func idemKey() string { return uuid.NewString() }

// ref resolves a reference item id by kind and code through the API.
func (c *client) ref(path, code string) string {
	var out struct {
		Items []struct {
			ID   string `json:"id"`
			Code string `json:"code"`
		} `json:"items"`
	}
	c.do("GET", "/api/v1/reference/"+path, nil, &out)
	for _, it := range out.Items {
		if it.Code == code {
			return it.ID
		}
	}
	c.e.t.Fatalf("reference %s/%s not found", path, code)
	return ""
}

func (c *client) courtID(code string) string {
	var out struct {
		Items []struct {
			ID   string `json:"id"`
			Code string `json:"code"`
		} `json:"items"`
	}
	c.do("GET", "/api/v1/reference/courts", nil, &out)
	for _, it := range out.Items {
		if it.Code == code {
			return it.ID
		}
	}
	c.e.t.Fatalf("court %s not found", code)
	return ""
}

// matterFixture creates a matter with one proceeding and one hearing as c,
// returning their ids.
type fixture struct {
	MatterID, ProceedingID, HearingID string
}

func (c *client) matter(title, confidentiality string, team []map[string]any, hearingDate string) fixture {
	c.e.t.Helper()
	var m struct {
		ID string `json:"id"`
	}
	st := c.do("POST", "/api/v1/matters", map[string]any{
		"title_en": title, "confidentiality": confidentiality,
		"clients": []any{map[string]any{"new": map[string]any{"kind": "person", "name_en": "Client of " + title}}},
		"team":    team,
	}, &m)
	if st != 201 {
		c.e.t.Fatalf("create matter %q: %d", title, st)
	}
	var pr struct {
		Data struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	st = c.do("POST", "/api/v1/matters/"+m.ID+"/proceedings", map[string]any{
		"proceeding_type_id": c.ref("proceeding-types", "dc_land"), "court_id": c.courtID("DC-COLOMBO"),
		"case_number": "L/" + strings.ToUpper(randHex(2)) + "/24",
		"parties": []any{
			map[string]any{"new": map[string]any{"kind": "person", "name_en": "Nimal Perera", "name_si": "නිමල් පෙරේරා"},
				"party_role_type_id": c.ref("party-role-types", "plaintiff"), "side": "our_client"},
			map[string]any{"new": map[string]any{"kind": "person", "name_en": "Kamal Silva"},
				"party_role_type_id": c.ref("party-role-types", "defendant"), "side": "opposing"},
		},
	}, &pr)
	if st != 201 {
		c.e.t.Fatalf("create proceeding: %d", st)
	}
	var h struct {
		Data struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	st = c.do("POST", "/api/v1/proceedings/"+pr.Data.ID+"/hearings", map[string]any{
		"date": hearingDate, "session": "morning", "source_type_id": c.ref("date-source-types", "lawyer_entry"),
		"purpose_id": c.ref("hearing-purposes", "trial"), "instructions": "Object to P9",
	}, &h)
	if st != 201 {
		c.e.t.Fatalf("create hearing: %d", st)
	}
	return fixture{MatterID: m.ID, ProceedingID: pr.Data.ID, HearingID: h.Data.ID}
}

func member(id uuid.UUID, role string) map[string]any {
	return map[string]any{"user_id": id.String(), "team_role": role}
}

func todayStr() string { return time.Now().In(time.FixedZone("LK", 5*3600+1800)).Format(time.DateOnly) }

func dayStr(offset int) string {
	return time.Now().In(time.FixedZone("LK", 5*3600+1800)).AddDate(0, 0, offset).Format(time.DateOnly)
}

// auditCount counts audit events for an action in a firm (as owner).
func (e *env) auditCount(firm, action string) int {
	var n int
	_ = e.owner.QueryRow(context.Background(), `select count(*) from audit_event where firm_id = $1 and action = $2`,
		e.firms[firm], action).Scan(&n)
	return n
}

func (e *env) verifyChain(firm string) {
	e.t.Helper()
	ctx := context.Background()
	err := pgx.BeginFunc(ctx, e.owner, func(tx pgx.Tx) error {
		bad, err := audit.VerifyChain(ctx, tx, e.firms[firm])
		if err != nil {
			return err
		}
		if bad != 0 {
			return fmt.Errorf("chain broken at %d", bad)
		}
		return nil
	})
	if err != nil {
		e.t.Fatalf("audit chain: %v", err)
	}
}
