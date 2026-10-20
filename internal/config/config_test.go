package config_test

import (
	"os"
	"strings"
	"testing"
	"time"

	"github.com/nipuna-cloudsecadvisors/NithiDesk/internal/config"
)

// valid is the minimum environment that produces a usable configuration. The
// values are synthetic and local-only.
func valid() map[string]string {
	return map[string]string{
		"NITHIDESK_DATABASE_URL":           "postgres://nithidesk:local-only@localhost:5432/nithidesk",
		"NITHIDESK_OBJECTSTORE_ENDPOINT":   "localhost:9000",
		"NITHIDESK_OBJECTSTORE_BUCKET":     "nithidesk-local",
		"NITHIDESK_OBJECTSTORE_ACCESS_KEY": "local-access-key",
		"NITHIDESK_OBJECTSTORE_SECRET_KEY": "local-secret-key",
		// 32 zero bytes, base64: synthetic and never used outside tests.
		"NITHIDESK_SECRETS_KEY": "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA=",
	}
}

func TestSecretsKeyMustBe32Bytes(t *testing.T) {
	env := valid()
	env["NITHIDESK_SECRETS_KEY"] = "dG9vLXNob3J0"
	setEnv(t, env)

	_, err := config.Load()
	if err == nil || !strings.Contains(err.Error(), "NITHIDESK_SECRETS_KEY") {
		t.Fatalf("Load() with a short key returned %v, want an error naming NITHIDESK_SECRETS_KEY", err)
	}
}

func TestFilesystemObjectStoreIsDevelopmentOnly(t *testing.T) {
	env := valid()
	env["NITHIDESK_OBJECTSTORE_DRIVER"] = "fs"
	env["NITHIDESK_OBJECTSTORE_FS_ROOT"] = t.TempDir()
	env["NITHIDESK_ENV"] = "production"
	setEnv(t, env)

	_, err := config.Load()
	if err == nil || !strings.Contains(err.Error(), "development only") {
		t.Fatalf("Load() with fs storage in production returned %v, want a refusal", err)
	}
}

func TestInsecureCookiesAreRefusedInProduction(t *testing.T) {
	env := valid()
	env["NITHIDESK_ENV"] = "production"
	env["NITHIDESK_COOKIE_SECURE"] = "false"
	setEnv(t, env)

	_, err := config.Load()
	if err == nil || !strings.Contains(err.Error(), "NITHIDESK_COOKIE_SECURE") {
		t.Fatalf("Load() with insecure cookies in production returned %v, want a refusal", err)
	}
}

func setEnv(t *testing.T, env map[string]string) {
	t.Helper()
	clearEnv(t)
	for key, value := range env {
		t.Setenv(key, value)
	}
}

func TestLoadAppliesDefaults(t *testing.T) {
	setEnv(t, valid())

	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("Load() returned %v, want no error", err)
	}
	if cfg.Env != "development" {
		t.Errorf("Env = %q, want development", cfg.Env)
	}
	if cfg.HTTPAddr != ":8080" {
		t.Errorf("HTTPAddr = %q, want :8080", cfg.HTTPAddr)
	}
	if cfg.Worker.PollInterval != 5*time.Second {
		t.Errorf("Worker.PollInterval = %v, want 5s", cfg.Worker.PollInterval)
	}
	if cfg.ObjectStore.Region != "us-east-1" {
		t.Errorf("ObjectStore.Region = %q, want us-east-1", cfg.ObjectStore.Region)
	}
	if cfg.ObjectStore.UseSSL {
		t.Error("ObjectStore.UseSSL = true, want false by default")
	}
}

// clearEnv blanks every NITHIDESK_ variable so a developer's shell (for
// example one with .data/dev.env loaded) cannot change a test's outcome.
func clearEnv(t *testing.T) {
	t.Helper()
	for _, kv := range os.Environ() {
		if key, _, ok := strings.Cut(kv, "="); ok && strings.HasPrefix(key, "NITHIDESK_") {
			t.Setenv(key, "")
		}
	}
}

func TestLoadReportsEveryMissingRequirementAtOnce(t *testing.T) {
	clearEnv(t)
	_, err := config.Load()
	if err == nil {
		t.Fatal("Load() with an empty environment returned no error")
	}
	for _, want := range []string{
		"NITHIDESK_DATABASE_URL",
		"NITHIDESK_OBJECTSTORE_ENDPOINT",
		"NITHIDESK_OBJECTSTORE_BUCKET",
		"NITHIDESK_OBJECTSTORE_ACCESS_KEY",
	} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error should name %s, got %v", want, err)
		}
	}
}

func TestLoadRejectsMalformedValues(t *testing.T) {
	env := valid()
	env["NITHIDESK_WORKER_POLL_INTERVAL"] = "soon"
	env["NITHIDESK_DATABASE_MAX_CONNS"] = "lots"
	setEnv(t, env)

	_, err := config.Load()
	if err == nil {
		t.Fatal("Load() with malformed values returned no error")
	}
	if !strings.Contains(err.Error(), "NITHIDESK_WORKER_POLL_INTERVAL") {
		t.Errorf("error should name the bad duration, got %v", err)
	}
	if !strings.Contains(err.Error(), "NITHIDESK_DATABASE_MAX_CONNS") {
		t.Errorf("error should name the bad integer, got %v", err)
	}
}

// Start-up logs one configuration line. It must not be a way to leak the
// database password or the object storage keys.
func TestLoggableRedactsSecrets(t *testing.T) {
	setEnv(t, valid())

	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("Load() returned %v, want no error", err)
	}

	loggable := cfg.Loggable()
	for _, key := range []string{"database_url", "objectstore_accesskey", "objectstore_secretkey"} {
		if loggable[key] != config.Redacted {
			t.Errorf("Loggable()[%q] = %v, want %q", key, loggable[key], config.Redacted)
		}
	}
	for key, value := range loggable {
		text, isString := value.(string)
		if !isString {
			continue
		}
		for _, secret := range []string{"local-only", "local-access-key", "local-secret-key"} {
			if strings.Contains(text, secret) {
				t.Errorf("Loggable()[%q] leaked a secret value", key)
			}
		}
	}
}
