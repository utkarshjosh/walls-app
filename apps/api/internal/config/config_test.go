package config

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// clearEnv unsets every WALLS_* variable for the duration of a test.
func clearEnv(t *testing.T) {
	t.Helper()
	for _, kv := range os.Environ() {
		if name, _, _ := strings.Cut(kv, "="); strings.HasPrefix(name, "WALLS_") {
			t.Setenv(name, "") // registers restore-on-cleanup
			os.Unsetenv(name)
		}
	}
}

func TestDefaults(t *testing.T) {
	clearEnv(t)
	c, err := FromEnv()
	if err != nil {
		t.Fatal(err)
	}
	if c.Addr != ":8787" || c.DataDir != "./data" || c.Storage != "local" || c.StorageCapBytes != 7_000_000_000 || c.PublicURL != "" {
		t.Errorf("defaults = %+v", c)
	}
	want := []string{"http://localhost:5173", "http://127.0.0.1:5173", "tauri://localhost", "http://tauri.localhost"}
	if !slices.Equal(c.CORSOrigins, want) {
		t.Errorf("default CORS origins = %v", c.CORSOrigins)
	}
}

func TestFromEnvValidation(t *testing.T) {
	clearEnv(t)
	t.Setenv("WALLS_STORAGE", "s3")
	_, err := FromEnv()
	if err == nil || !strings.Contains(err.Error(), "WALLS_S3_ENDPOINT") || !strings.Contains(err.Error(), "WALLS_S3_SECRET_KEY") {
		t.Fatalf("s3 without settings: %v", err)
	}

	t.Setenv("WALLS_S3_ENDPOINT", "https://ns.compat.objectstorage.ap-mumbai-1.oraclecloud.com")
	t.Setenv("WALLS_S3_REGION", "ap-mumbai-1")
	t.Setenv("WALLS_S3_BUCKET", "walls-assets")
	t.Setenv("WALLS_S3_ACCESS_KEY", "ak")
	t.Setenv("WALLS_S3_SECRET_KEY", "sk")
	t.Setenv("WALLS_CORS_ORIGINS", " https://a.example/ , ,tauri://localhost")
	t.Setenv("WALLS_PUBLIC_URL", "https://walls.example.com/")
	t.Setenv("WALLS_STORAGE_CAP_BYTES", "12345")
	c, err := FromEnv()
	if err != nil {
		t.Fatal(err)
	}
	if c.Storage != "s3" || c.S3Bucket != "walls-assets" || c.PublicURL != "https://walls.example.com" || c.StorageCapBytes != 12345 ||
		!slices.Equal(c.CORSOrigins, []string{"https://a.example", "tauri://localhost"}) {
		t.Errorf("config = %+v", c)
	}

	for name, bad := range map[string]string{
		"WALLS_STORAGE": "ftp", "WALLS_STORAGE_CAP_BYTES": "lots", "WALLS_PUBLIC_URL": "walls.example.com",
	} {
		t.Run(name, func(t *testing.T) {
			t.Setenv(name, bad)
			if _, err := FromEnv(); err == nil || !strings.Contains(err.Error(), name) {
				t.Errorf("%s=%q: got %v", name, bad, err)
			}
		})
	}
}

func TestLoadDotEnv(t *testing.T) {
	clearEnv(t)
	path := filepath.Join(t.TempDir(), ".env")
	os.WriteFile(path, []byte(`# a comment

WALLS_ADDR=:9999
export WALLS_S3_BUCKET = "walls-assets"
WALLS_S3_REGION='ap-mumbai-1'
WALLS_DATA_DIR=./somewhere # trailing comment
WALLS_S3_SECRET_KEY=abc#def=ghi
WALLS_PUBLIC_URL=
`), 0o600)

	t.Setenv("WALLS_ADDR", ":1234") // already set: the real environment wins
	if err := LoadDotEnv(path); err != nil {
		t.Fatal(err)
	}
	for name, want := range map[string]string{
		"WALLS_ADDR": ":1234", "WALLS_S3_BUCKET": "walls-assets", "WALLS_S3_REGION": "ap-mumbai-1",
		"WALLS_DATA_DIR": "./somewhere", "WALLS_S3_SECRET_KEY": "abc#def=ghi", "WALLS_PUBLIC_URL": "",
	} {
		if got := os.Getenv(name); got != want {
			t.Errorf("%s = %q, want %q", name, got, want)
		}
	}

	if err := LoadDotEnv(filepath.Join(t.TempDir(), "missing.env")); err != nil {
		t.Errorf("a missing .env should be fine, got %v", err)
	}

	// A malformed line is reported by line number, never by content.
	bad := filepath.Join(t.TempDir(), ".env")
	os.WriteFile(bad, []byte("WALLS_ADDR=:1\nthis-is-a-secret-without-equals\n"), 0o600)
	err := LoadDotEnv(bad)
	if err == nil || !strings.Contains(err.Error(), "line 2") || strings.Contains(err.Error(), "secret") {
		t.Errorf("malformed .env: %v", err)
	}
}

func TestMailSettings(t *testing.T) {
	clearEnv(t)
	c, err := FromEnv()
	if err != nil || c.MailEnabled() {
		t.Fatalf("with nothing set: mail on = %v, err = %v", c.MailEnabled(), err)
	}

	t.Setenv("WALLS_SMTP_HOST", "smtp.example.com")
	if _, err := FromEnv(); err == nil || !strings.Contains(err.Error(), "WALLS_MAIL_FROM") {
		t.Errorf("a relay with no From address: err = %v", err)
	}
	t.Setenv("WALLS_MAIL_FROM", "Still <hello@still.example>")
	c, err = FromEnv()
	if err != nil || !c.MailEnabled() || c.SMTPPort != 587 || c.MailToLog {
		t.Fatalf("relay settings = %+v, %v", c, err)
	}
	t.Setenv("WALLS_SMTP_USER", "apikey")
	if _, err := FromEnv(); err == nil {
		t.Error("a user name with no password was accepted")
	}
	t.Setenv("WALLS_SMTP_PASSWORD", "s3cret")
	t.Setenv("WALLS_SMTP_PORT", "465")
	if c, err = FromEnv(); err != nil || c.SMTPPort != 465 || c.SMTPPassword != "s3cret" {
		t.Fatalf("full relay settings = %+v, %v", c, err)
	}
	for name, bad := range map[string]string{"WALLS_SMTP_PORT": "smtp", "WALLS_MAIL_FROM": "nobody", "WALLS_MAIL_TO_LOG": "maybe"} {
		t.Run(name, func(t *testing.T) {
			t.Setenv(name, bad)
			if _, err := FromEnv(); err == nil {
				t.Errorf("%s=%q was accepted", name, bad)
			}
		})
	}

	clearEnv(t)
	t.Setenv("WALLS_MAIL_TO_LOG", "1")
	if c, err = FromEnv(); err != nil || !c.MailEnabled() || !c.MailToLog {
		t.Errorf("mail to the log = %+v, %v", c, err)
	}
}
