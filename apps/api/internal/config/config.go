// Package config reads the service settings from environment variables,
// optionally pre-loaded from a .env file in the working directory.
package config

import (
	"bufio"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"strconv"
	"strings"
)

// DefaultCORSOrigins covers the Vite dev server and the Tauri desktop shell.
const DefaultCORSOrigins = "http://localhost:5173,http://127.0.0.1:5173,tauri://localhost,http://tauri.localhost"

// DefaultStorageCap is the owner's bucket budget in bytes.
const DefaultStorageCap int64 = 7_000_000_000

// Config is the full runtime configuration.
type Config struct {
	Addr            string
	DataDir         string
	Storage         string // "local" or "s3"
	S3Endpoint      string
	S3Region        string
	S3Bucket        string
	S3AccessKey     string
	S3SecretKey     string
	CORSOrigins     []string
	PublicURL       string // optional absolute base for file URLs, no trailing slash
	StorageCapBytes int64
	// AdminPassword is the owner's password for the admin portal at /admin.
	// Empty leaves the portal switched off.
	AdminPassword string
}

// MinAdminPassword is the shortest admin password accepted. The portal is on
// the public internet, so a short password is refused rather than trusted.
const MinAdminPassword = 12

// LoadDotEnv reads KEY=VALUE lines from path into the process environment.
// Variables that are already set win, so real environment values override the
// file. A missing file is not an error.
func LoadDotEnv(path string) error {
	f, err := os.Open(path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil
		}
		return err
	}
	defer f.Close()

	sc := bufio.NewScanner(f)
	for n := 1; sc.Scan(); n++ {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		line = strings.TrimPrefix(line, "export ")
		key, val, ok := strings.Cut(line, "=")
		key = strings.TrimSpace(key)
		if !ok || key == "" || strings.ContainsAny(key, " \t") {
			// Never echo the line itself: it may hold a secret.
			return fmt.Errorf("%s line %d: expected KEY=VALUE", path, n)
		}
		val = strings.TrimSpace(val)
		if len(val) >= 2 && (val[0] == '"' || val[0] == '\'') && val[len(val)-1] == val[0] {
			val = val[1 : len(val)-1]
		} else if i := strings.Index(val, " #"); i >= 0 {
			val = strings.TrimSpace(val[:i]) // trailing comment on an unquoted value
		}
		if _, set := os.LookupEnv(key); !set {
			os.Setenv(key, val)
		}
	}
	return sc.Err()
}

func env(key, def string) string {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		return v
	}
	return def
}

// FromEnv builds a Config from WALLS_* variables and checks it.
func FromEnv() (Config, error) {
	c := Config{
		Addr:        env("WALLS_ADDR", ":8787"),
		DataDir:     env("WALLS_DATA_DIR", "./data"),
		Storage:     strings.ToLower(env("WALLS_STORAGE", "local")),
		S3Endpoint:  env("WALLS_S3_ENDPOINT", ""),
		S3Region:    env("WALLS_S3_REGION", ""),
		S3Bucket:    env("WALLS_S3_BUCKET", ""),
		S3AccessKey: env("WALLS_S3_ACCESS_KEY", ""),
		S3SecretKey: env("WALLS_S3_SECRET_KEY", ""),
		PublicURL:   strings.TrimRight(env("WALLS_PUBLIC_URL", ""), "/"),
		// Not trimmed or defaulted: a password is used exactly as written.
		AdminPassword: os.Getenv("WALLS_ADMIN_PASSWORD"),
	}
	if n := len(c.AdminPassword); n > 0 && n < MinAdminPassword {
		return c, fmt.Errorf("WALLS_ADMIN_PASSWORD must be at least %d characters (or left out, which switches the admin portal off)", MinAdminPassword)
	}
	for _, o := range strings.Split(env("WALLS_CORS_ORIGINS", DefaultCORSOrigins), ",") {
		if o = strings.TrimRight(strings.TrimSpace(o), "/"); o != "" {
			c.CORSOrigins = append(c.CORSOrigins, o)
		}
	}

	c.StorageCapBytes = DefaultStorageCap
	if v := env("WALLS_STORAGE_CAP_BYTES", ""); v != "" {
		n, err := strconv.ParseInt(v, 10, 64)
		if err != nil || n <= 0 {
			return c, fmt.Errorf("WALLS_STORAGE_CAP_BYTES must be a positive whole number of bytes, got %q", v)
		}
		c.StorageCapBytes = n
	}

	switch c.Storage {
	case "local":
	case "s3":
		var missing []string
		for _, kv := range [][2]string{
			{"WALLS_S3_ENDPOINT", c.S3Endpoint},
			{"WALLS_S3_REGION", c.S3Region},
			{"WALLS_S3_BUCKET", c.S3Bucket},
			{"WALLS_S3_ACCESS_KEY", c.S3AccessKey},
			{"WALLS_S3_SECRET_KEY", c.S3SecretKey},
		} {
			if kv[1] == "" {
				missing = append(missing, kv[0])
			}
		}
		if len(missing) > 0 {
			return c, fmt.Errorf("WALLS_STORAGE=s3 needs %s", strings.Join(missing, ", "))
		}
	default:
		return c, fmt.Errorf("WALLS_STORAGE must be \"local\" or \"s3\", got %q", c.Storage)
	}
	if c.PublicURL != "" && !strings.HasPrefix(c.PublicURL, "http://") && !strings.HasPrefix(c.PublicURL, "https://") {
		return c, fmt.Errorf("WALLS_PUBLIC_URL must start with http:// or https://")
	}
	return c, nil
}
