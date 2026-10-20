// Command nithidesk is the operator tool: schema migrations, the runtime
// database login, firm bootstrap, user creation and audit-chain checks.
//
// It connects with NITHIDESK_MIGRATE_DATABASE_URL, the schema owner. The API
// and worker never use that connection: they connect as a member of
// nithidesk_app, which row-level security always applies to.
//
//	nithidesk migrate
//	nithidesk create-app-login -name nithidesk_api            (password from NITHIDESK_APP_DB_PASSWORD)
//	nithidesk create-firm -code demo -name-en "Demo Chambers" [-name-si ...]
//	nithidesk create-user -firm demo -email a@b.lk -name "Admin" -roles system_administrator   (password from NITHIDESK_USER_PASSWORD)
//	nithidesk verify-audit -firm demo
//	nithidesk seed-demo -firm demo                             (synthetic data for development only)
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"regexp"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/nipuna-cloudsecadvisors/NithiDesk/internal/audit"
	"github.com/nipuna-cloudsecadvisors/NithiDesk/internal/authz"
	"github.com/nipuna-cloudsecadvisors/NithiDesk/internal/courts"
	"github.com/nipuna-cloudsecadvisors/NithiDesk/internal/identity"
	"github.com/nipuna-cloudsecadvisors/NithiDesk/internal/platform/migrate"
	"github.com/nipuna-cloudsecadvisors/NithiDesk/migrations"
)

func main() {
	if len(os.Args) < 2 {
		usage()
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()
	var err error
	switch os.Args[1] {
	case "migrate":
		err = runMigrate(ctx)
	case "create-app-login":
		err = createAppLogin(ctx, os.Args[2:])
	case "create-firm":
		err = createFirm(ctx, os.Args[2:])
	case "create-user":
		err = createUser(ctx, os.Args[2:])
	case "verify-audit":
		err = verifyAudit(ctx, os.Args[2:])
	case "seed-demo":
		err = seedDemo(ctx, os.Args[2:])
	default:
		usage()
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

func usage() {
	fmt.Fprintln(os.Stderr, "usage: nithidesk migrate | create-app-login | create-firm | create-user | verify-audit | seed-demo")
	os.Exit(2)
}

func ownerConn(ctx context.Context) (*pgx.Conn, error) {
	url := os.Getenv("NITHIDESK_MIGRATE_DATABASE_URL")
	if url == "" {
		return nil, errors.New("NITHIDESK_MIGRATE_DATABASE_URL is required")
	}
	conn, err := pgx.Connect(ctx, url)
	if err != nil {
		// The URL holds a password; never echo it.
		return nil, errors.New("cannot connect with NITHIDESK_MIGRATE_DATABASE_URL")
	}
	return conn, nil
}

func runMigrate(ctx context.Context) error {
	conn, err := ownerConn(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = conn.Close(ctx) }()
	ms, err := migrate.Load(migrations.FS)
	if err != nil {
		return err
	}
	if len(ms) == 0 {
		return migrate.ErrNoMigrations
	}
	res, err := migrate.Up(ctx, conn, ms)
	if err != nil {
		return err
	}
	for _, m := range res.Applied {
		fmt.Printf("applied %04d_%s\n", m.Version, m.Name)
	}
	fmt.Printf("schema at version %04d\n", res.Current)
	return nil
}

var identPattern = regexp.MustCompile(`^[a-z][a-z0-9_]{2,40}$`)

func createAppLogin(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("create-app-login", flag.ExitOnError)
	name := fs.String("name", "nithidesk_api", "login role name")
	_ = fs.Parse(args)
	password := os.Getenv("NITHIDESK_APP_DB_PASSWORD")
	if !identPattern.MatchString(*name) || len(password) < 16 {
		return errors.New("need a lower-case role name and NITHIDESK_APP_DB_PASSWORD of at least 16 characters")
	}
	conn, err := ownerConn(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = conn.Close(ctx) }()
	ident := pgx.Identifier{*name}.Sanitize()
	var exists bool
	if err := conn.QueryRow(ctx, `select exists (select 1 from pg_roles where rolname = $1)`, *name).Scan(&exists); err != nil {
		return err
	}
	// CREATE/ALTER ROLE cannot take a bind parameter for the password, so
	// quote it as a literal with doubled single quotes.
	lit := "'" + strings.ReplaceAll(password, "'", "''") + "'"
	stmt := "create role " + ident + " login password " + lit + " in role nithidesk_app"
	if exists {
		stmt = "alter role " + ident + " login password " + lit
	}
	if _, err := conn.Exec(ctx, stmt); err != nil {
		return fmt.Errorf("create login: %w", err)
	}
	if exists {
		if _, err := conn.Exec(ctx, "grant nithidesk_app to "+ident); err != nil {
			return fmt.Errorf("grant role: %w", err)
		}
	}
	fmt.Println("login", *name, "ready (member of nithidesk_app)")
	return nil
}

func createFirm(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("create-firm", flag.ExitOnError)
	code := fs.String("code", "", "firm code (lower case)")
	nameEN := fs.String("name-en", "", "firm name in English")
	nameSI := fs.String("name-si", "", "firm name in Sinhala")
	locale := fs.String("locale", "en", "default UI locale (en or si)")
	_ = fs.Parse(args)
	if *code == "" || (*nameEN == "" && *nameSI == "") {
		return errors.New("-code and a name are required")
	}
	conn, err := ownerConn(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = conn.Close(ctx) }()
	return pgx.BeginFunc(ctx, conn, func(tx pgx.Tx) error {
		var firmID uuid.UUID
		err := tx.QueryRow(ctx, `insert into firm (code, name_en, name_si, default_locale) values ($1, $2, $3, $4) returning id`,
			strings.ToLower(*code), nullable(*nameEN), nullable(*nameSI), *locale).Scan(&firmID)
		if err != nil {
			return fmt.Errorf("create firm: %w", err)
		}
		for _, rt := range authz.DefaultRoleTemplates {
			caps := make([]string, len(rt.Capabilities))
			for i, c := range rt.Capabilities {
				caps[i] = string(c)
			}
			if _, err := tx.Exec(ctx, `insert into role_template (firm_id, code, name_en, name_si, capabilities, matter_scope, is_system)
				values ($1, $2, $3, $4, $5, $6, true)`, firmID, rt.Code, rt.NameEN, rt.NameSI, caps, rt.MatterScope); err != nil {
				return fmt.Errorf("seed role %s: %w", rt.Code, err)
			}
		}
		if err := courts.SeedFirm(ctx, tx, firmID); err != nil {
			return err
		}
		if err := audit.Append(ctx, tx, audit.Event{FirmID: firmID, Actor: audit.Actor{Kind: audit.ActorSystem},
			Action: "firm.created", TargetKind: "firm", TargetID: firmID}); err != nil {
			return err
		}
		fmt.Printf("firm %s created (%s) with %d role templates and default reference data\n", *code, firmID,
			len(authz.DefaultRoleTemplates))
		return nil
	})
}

func createUser(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("create-user", flag.ExitOnError)
	firm := fs.String("firm", "", "firm code")
	email := fs.String("email", "", "email")
	name := fs.String("name", "", "display name (English)")
	nameSI := fs.String("name-si", "", "display name (Sinhala)")
	roles := fs.String("roles", "", "comma-separated role template codes")
	locale := fs.String("locale", "en", "preferred locale")
	_ = fs.Parse(args)
	password := os.Getenv("NITHIDESK_USER_PASSWORD")
	if *firm == "" || *email == "" || (*name == "" && *nameSI == "") || *roles == "" {
		return errors.New("-firm, -email, -name and -roles are required")
	}
	if code := identity.CheckPasswordPolicy(password, *email); code != "" {
		return fmt.Errorf("NITHIDESK_USER_PASSWORD rejected: %s", code)
	}
	conn, err := ownerConn(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = conn.Close(ctx) }()
	hash, err := identity.HashPassword(password)
	if err != nil {
		return err
	}
	return pgx.BeginFunc(ctx, conn, func(tx pgx.Tx) error {
		userID, err := insertUser(ctx, tx, *firm, strings.ToLower(*email), *name, *nameSI, *locale, hash, strings.Split(*roles, ","))
		if err != nil {
			return err
		}
		fmt.Printf("user %s created (%s); they enrol TOTP at first login\n", *email, userID)
		return nil
	})
}

func insertUser(ctx context.Context, tx pgx.Tx, firm, email, name, nameSI, locale, hash string, roles []string) (uuid.UUID, error) {
	var firmID, userID uuid.UUID
	if err := tx.QueryRow(ctx, `select id from firm where code = $1`, firm).Scan(&firmID); err != nil {
		return uuid.Nil, fmt.Errorf("unknown firm %q", firm)
	}
	if err := tx.QueryRow(ctx, `insert into app_user (firm_id, email, display_name_en, display_name_si, preferred_locale,
		status, password_hash, password_changed_at) values ($1, $2, $3, $4, $5, 'invited', $6, now()) returning id`,
		firmID, email, nullable(name), nullable(nameSI), locale, hash).Scan(&userID); err != nil {
		return uuid.Nil, fmt.Errorf("create user: %w", err)
	}
	for _, role := range roles {
		tag, err := tx.Exec(ctx, `insert into user_role (firm_id, user_id, role_template_id)
			select $1, $2, id from role_template where firm_id = $1 and code = $3`, firmID, userID, strings.TrimSpace(role))
		if err != nil {
			return uuid.Nil, err
		}
		if tag.RowsAffected() == 0 {
			return uuid.Nil, fmt.Errorf("unknown role %q", role)
		}
	}
	return userID, audit.Append(ctx, tx, audit.Event{FirmID: firmID, Actor: audit.Actor{Kind: audit.ActorSystem},
		Action: "user.created", TargetKind: "user", TargetID: userID, Changes: map[string]any{"roles": roles, "via": "cli"}})
}

func verifyAudit(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("verify-audit", flag.ExitOnError)
	firm := fs.String("firm", "", "firm code")
	_ = fs.Parse(args)
	conn, err := ownerConn(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = conn.Close(ctx) }()
	return pgx.BeginFunc(ctx, conn, func(tx pgx.Tx) error {
		var firmID uuid.UUID
		if err := tx.QueryRow(ctx, `select id from firm where code = $1`, *firm).Scan(&firmID); err != nil {
			return fmt.Errorf("unknown firm %q", *firm)
		}
		bad, err := audit.VerifyChain(ctx, tx, firmID)
		if err != nil {
			return err
		}
		if bad != 0 {
			return fmt.Errorf("audit chain broken at event %d", bad)
		}
		var n int
		_ = tx.QueryRow(ctx, `select count(*) from audit_event where firm_id = $1`, firmID).Scan(&n)
		fmt.Printf("audit chain intact (%d events)\n", n)
		return nil
	})
}

func nullable(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}
