// Package config loads runtime configuration from the environment.
//
// Every value comes from the environment. Nothing is read from a committed
// file and nothing has a hosting provider baked into it, so the same binary
// runs locally, in CI, and in whichever environment the firm chooses later.
package config

import (
	"encoding/base64"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

// Config is the full runtime configuration for the API and the worker.
type Config struct {
	Env             string
	HTTPAddr        string
	ShutdownTimeout time.Duration

	// DefaultFirm is the firm code used when a login request names none. A
	// single-firm deployment (ADR 0002) sets it and clients never send one.
	DefaultFirm string

	// SecretsKey encrypts secrets at rest in the database, such as TOTP
	// seeds. It is 32 bytes, supplied base64-encoded.
	SecretsKey []byte

	Database    Database
	ObjectStore ObjectStore
	Worker      Worker
	Session     Session
	Web         Web
	PDF         PDF
}

// Database describes the PostgreSQL connection.
type Database struct {
	URL              string
	MaxConns         int32
	ConnectTimeout   time.Duration
	StatementTimeout time.Duration
}

// ObjectStore describes private object storage. Only the S3 API is used in
// hosted environments, so MinIO locally and any S3-compatible service are
// interchangeable. The "fs" driver keeps objects in a local directory for
// development machines without MinIO; it is refused in production.
type ObjectStore struct {
	Driver    string
	Endpoint  string
	Region    string
	Bucket    string
	AccessKey string
	SecretKey string
	UseSSL    bool
	FSRoot    string
}

// Worker describes the background worker loop. The queue is PostgreSQL-backed,
// which keeps the local stack to one datastore.
type Worker struct {
	PollInterval time.Duration
	BatchSize    int
}

// Session describes the server-side session policy (ADR 0004, doc 10 §4).
type Session struct {
	CookieSecure   bool
	IdleDesktop    time.Duration
	IdleMobile     time.Duration
	Absolute       time.Duration
	StepUpValidity time.Duration
}

// Web describes the static single-page application served on the API origin
// (ADR 0008). Empty means the API serves no UI.
type Web struct {
	StaticDir string
}

// PDF describes the headless Chromium renderer used for printable briefs
// (ADR 0007). FontsDir holds the bundled static Noto fonts.
type PDF struct {
	ChromePath    string
	FontsDir      string
	RenderTimeout time.Duration
}

// Redacted secret values are replaced with this marker when configuration is
// logged, so a credential cannot reach the log pipeline through a config dump.
const Redacted = "[redacted]"

// Load reads configuration from the environment and validates it. It returns
// every problem at once so a misconfigured environment is fixed in one pass.
func Load() (Config, error) {
	var problems []string
	note := func(format string, args ...any) {
		problems = append(problems, fmt.Sprintf(format, args...))
	}

	cfg := Config{
		Env:             optional("NITHIDESK_ENV", "development"),
		HTTPAddr:        optional("NITHIDESK_HTTP_ADDR", ":8080"),
		ShutdownTimeout: duration("NITHIDESK_SHUTDOWN_TIMEOUT", 15*time.Second, note),
		DefaultFirm:     strings.ToLower(optional("NITHIDESK_DEFAULT_FIRM", "")),
		Database: Database{
			URL:              os.Getenv("NITHIDESK_DATABASE_URL"),
			MaxConns:         int32(integer("NITHIDESK_DATABASE_MAX_CONNS", 10, note)),
			ConnectTimeout:   duration("NITHIDESK_DATABASE_CONNECT_TIMEOUT", 5*time.Second, note),
			StatementTimeout: duration("NITHIDESK_DATABASE_STATEMENT_TIMEOUT", 10*time.Second, note),
		},
		ObjectStore: ObjectStore{
			Driver:    strings.ToLower(optional("NITHIDESK_OBJECTSTORE_DRIVER", "s3")),
			Endpoint:  os.Getenv("NITHIDESK_OBJECTSTORE_ENDPOINT"),
			Region:    optional("NITHIDESK_OBJECTSTORE_REGION", "us-east-1"),
			Bucket:    os.Getenv("NITHIDESK_OBJECTSTORE_BUCKET"),
			AccessKey: os.Getenv("NITHIDESK_OBJECTSTORE_ACCESS_KEY"),
			SecretKey: os.Getenv("NITHIDESK_OBJECTSTORE_SECRET_KEY"),
			UseSSL:    boolean("NITHIDESK_OBJECTSTORE_USE_SSL", false, note),
			FSRoot:    os.Getenv("NITHIDESK_OBJECTSTORE_FS_ROOT"),
		},
		Worker: Worker{
			PollInterval: duration("NITHIDESK_WORKER_POLL_INTERVAL", 5*time.Second, note),
			BatchSize:    integer("NITHIDESK_WORKER_BATCH_SIZE", 10, note),
		},
		Session: Session{
			CookieSecure:   boolean("NITHIDESK_COOKIE_SECURE", true, note),
			IdleDesktop:    duration("NITHIDESK_SESSION_IDLE_DESKTOP", 30*time.Minute, note),
			IdleMobile:     duration("NITHIDESK_SESSION_IDLE_MOBILE", 15*time.Minute, note),
			Absolute:       duration("NITHIDESK_SESSION_ABSOLUTE", 12*time.Hour, note),
			StepUpValidity: duration("NITHIDESK_STEP_UP_VALIDITY", 5*time.Minute, note),
		},
		Web: Web{
			StaticDir: os.Getenv("NITHIDESK_WEB_DIR"),
		},
		PDF: PDF{
			ChromePath:    os.Getenv("NITHIDESK_CHROME_PATH"),
			FontsDir:      os.Getenv("NITHIDESK_FONTS_DIR"),
			RenderTimeout: duration("NITHIDESK_PDF_RENDER_TIMEOUT", 60*time.Second, note),
		},
	}

	if cfg.Database.URL == "" {
		note("NITHIDESK_DATABASE_URL is required")
	}

	switch cfg.ObjectStore.Driver {
	case "s3":
		if cfg.ObjectStore.Endpoint == "" {
			note("NITHIDESK_OBJECTSTORE_ENDPOINT is required")
		}
		if cfg.ObjectStore.Bucket == "" {
			note("NITHIDESK_OBJECTSTORE_BUCKET is required")
		}
		if cfg.ObjectStore.AccessKey == "" || cfg.ObjectStore.SecretKey == "" {
			note("NITHIDESK_OBJECTSTORE_ACCESS_KEY and NITHIDESK_OBJECTSTORE_SECRET_KEY are required")
		}
	case "fs":
		if cfg.ObjectStore.FSRoot == "" {
			note("NITHIDESK_OBJECTSTORE_FS_ROOT is required when NITHIDESK_OBJECTSTORE_DRIVER=fs")
		}
		if cfg.Env == "production" {
			note("NITHIDESK_OBJECTSTORE_DRIVER=fs is for development only; use s3 in production")
		}
		if cfg.ObjectStore.Bucket == "" {
			cfg.ObjectStore.Bucket = "local"
		}
	default:
		note("NITHIDESK_OBJECTSTORE_DRIVER must be s3 or fs, got %q", cfg.ObjectStore.Driver)
	}

	rawKey := strings.TrimSpace(os.Getenv("NITHIDESK_SECRETS_KEY"))
	if rawKey == "" {
		note("NITHIDESK_SECRETS_KEY is required (32 random bytes, base64)")
	} else if key, err := base64.StdEncoding.DecodeString(rawKey); err != nil || len(key) != 32 {
		note("NITHIDESK_SECRETS_KEY must be 32 bytes encoded as base64")
	} else {
		cfg.SecretsKey = key
	}

	if cfg.Database.MaxConns < 1 {
		note("NITHIDESK_DATABASE_MAX_CONNS must be at least 1")
	}
	if cfg.Worker.BatchSize < 1 {
		note("NITHIDESK_WORKER_BATCH_SIZE must be at least 1")
	}
	if cfg.Env == "production" && !cfg.Session.CookieSecure {
		note("NITHIDESK_COOKIE_SECURE cannot be false in production")
	}
	if cfg.Session.IdleMobile <= 0 || cfg.Session.IdleDesktop <= 0 || cfg.Session.Absolute <= 0 {
		note("session timeouts must be positive")
	}

	if len(problems) > 0 {
		return Config{}, fmt.Errorf("invalid configuration: %w", errors.New(strings.Join(problems, "; ")))
	}
	return cfg, nil
}

// Loggable returns the configuration with every secret removed, for the one
// structured log line written at start-up. See docs/10-security-plan.md §7.
func (c Config) Loggable() map[string]any {
	return map[string]any{
		"env":                   c.Env,
		"http_addr":             c.HTTPAddr,
		"database_max_conns":    c.Database.MaxConns,
		"objectstore_endpoint":  c.ObjectStore.Endpoint,
		"objectstore_bucket":    c.ObjectStore.Bucket,
		"objectstore_use_ssl":   c.ObjectStore.UseSSL,
		"worker_poll_interval":  c.Worker.PollInterval.String(),
		"database_url":          Redacted,
		"objectstore_accesskey": Redacted,
		"objectstore_secretkey": Redacted,
		"secrets_key":           Redacted,
	}
}

func optional(key, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(key)); value != "" {
		return value
	}
	return fallback
}

func duration(key string, fallback time.Duration, note func(string, ...any)) time.Duration {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return fallback
	}
	parsed, err := time.ParseDuration(raw)
	if err != nil {
		note("%s must be a duration such as 10s, got %q", key, raw)
		return fallback
	}
	return parsed
}

func integer(key string, fallback int, note func(string, ...any)) int {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return fallback
	}
	parsed, err := strconv.Atoi(raw)
	if err != nil {
		note("%s must be an integer, got %q", key, raw)
		return fallback
	}
	return parsed
}

func boolean(key string, fallback bool, note func(string, ...any)) bool {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return fallback
	}
	parsed, err := strconv.ParseBool(raw)
	if err != nil {
		note("%s must be true or false, got %q", key, raw)
		return fallback
	}
	return parsed
}
