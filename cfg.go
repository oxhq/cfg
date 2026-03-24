// Package cfg loads configuration from YAML files, .env files, and environment
// variables into Go structs — without reflection, tags, or codegen.
//
// The user's struct opts in to each layer by implementing simple interfaces:
//
//	type Defaulter interface { Defaults() }
//	type EnvLoader interface { FromEnv() }
//	type Validator interface { Validate() error }
//
// The loading order is opinionated: Defaults → YAML → .env file → FromEnv → Validate.
// Environment variables always win.
//
// Usage:
//
//	type Config struct {
//	    Port  int
//	    Host  string
//	    Debug bool
//	}
//
//	func (c *Config) Defaults()        { c.Port = 8080; c.Host = "localhost" }
//	func (c *Config) FromEnv()         { c.Port = cfg.Int("PORT", c.Port); c.Host = cfg.Str("HOST", c.Host) }
//	func (c *Config) Validate() error  { if c.Port < 1 { return fmt.Errorf("invalid port") }; return nil }
//
//	var c Config
//	if err := cfg.New().Load(&c); err != nil { log.Fatal(err) }
package cfg

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// ---------------------------------------------------------------------------
// Interfaces — implement any combination on your config struct.
// ---------------------------------------------------------------------------

// Defaulter sets default values before any external source is applied.
type Defaulter interface{ Defaults() }

// EnvLoader reads environment variables into struct fields.
// Use the package-level helpers (Str, Int, Bool, etc.) inside this method.
type EnvLoader interface{ FromEnv() }

// Validator checks the final config for correctness.
type Validator interface{ Validate() error }

// ---------------------------------------------------------------------------
// Loader — builder that orchestrates the loading pipeline.
// ---------------------------------------------------------------------------

// Loader configures which files to read and in what order.
type Loader struct {
	rootPath    string
	yamlPath    string
	envFilePath string
	noYaml      bool
	noEnvFile   bool
	verbose     bool
	envStore    map[string]string // isolated .env values, no os.Setenv
}

// New creates a Loader with sensible defaults.
func New() *Loader {
	return &Loader{
		rootPath:    ".",
		yamlPath:    "config.yml",
		envFilePath: ".env",
		envStore:    make(map[string]string),
	}
}

func (l *Loader) WithRoot(root string) *Loader    { l.rootPath = root; return l }
func (l *Loader) WithYaml(path string) *Loader     { l.yamlPath = path; return l }
func (l *Loader) WithEnvFile(path string) *Loader   { l.envFilePath = path; return l }
func (l *Loader) NoYaml() *Loader                   { l.noYaml = true; return l }
func (l *Loader) NoEnvFile() *Loader                { l.noEnvFile = true; return l }
func (l *Loader) Verbose() *Loader                  { l.verbose = true; return l }


// Load runs the full pipeline: Defaults → YAML → .env → FromEnv → Validate.
// cfg must be a pointer to a struct that has yaml tags for YAML unmarshaling.
func (l *Loader) Load(cfg any) error {
	// 1. Defaults
	if d, ok := cfg.(Defaulter); ok {
		d.Defaults()
		l.log("applied Defaults()")
	}

	// 2. YAML
	if !l.noYaml {
		if err := l.loadYaml(cfg); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("cfg: yaml: %w", err)
		}
	}

	// 3. .env file (into isolated store, no os.Setenv)
	if !l.noEnvFile {
		if err := l.loadEnvFile(); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("cfg: envfile: %w", err)
		}
	}

	// 4. Expose the store so helpers can read from it, then call FromEnv
	activeLoader = l
	if e, ok := cfg.(EnvLoader); ok {
		e.FromEnv()
		l.log("applied FromEnv()")
	}
	activeLoader = nil

	// 5. Validate
	if v, ok := cfg.(Validator); ok {
		if err := v.Validate(); err != nil {
			return fmt.Errorf("cfg: validate: %w", err)
		}
		l.log("passed Validate()")
	}

	return nil
}

func (l *Loader) absPath(rel string) string {
	if filepath.IsAbs(rel) {
		return rel
	}
	return filepath.Join(l.rootPath, rel)
}

func (l *Loader) log(msg string) {
	if l.verbose {
		fmt.Fprintf(os.Stderr, "[cfg] %s\n", msg)
	}
}


// ---------------------------------------------------------------------------
// YAML loader — uses standard yaml tags, no custom reflection.
// ---------------------------------------------------------------------------

func (l *Loader) loadYaml(cfg any) error {
	path := l.absPath(l.yamlPath)
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	l.log("loaded yaml: " + path)
	return yaml.Unmarshal(data, cfg)
}

// ---------------------------------------------------------------------------
// .env file parser — robust, no os.Setenv side effects.
// ---------------------------------------------------------------------------

func (l *Loader) loadEnvFile() error {
	path := l.absPath(l.envFilePath)
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}

	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || line[0] == '#' {
			continue
		}

		// Strip optional "export " prefix.
		line = strings.TrimPrefix(line, "export ")

		eq := strings.IndexByte(line, '=')
		if eq < 1 {
			continue
		}
		key := strings.TrimSpace(line[:eq])
		val := strings.TrimSpace(line[eq+1:])

		// Strip matching quotes (single or double).
		if len(val) >= 2 {
			if (val[0] == '"' && val[len(val)-1] == '"') ||
				(val[0] == '\'' && val[len(val)-1] == '\'') {
				val = val[1 : len(val)-1]
			}
		}

		l.envStore[key] = val
		l.log(fmt.Sprintf("envfile: %s=%s", key, val))
	}

	l.log("loaded envfile: " + path)
	return nil
}


// ---------------------------------------------------------------------------
// activeLoader — set during FromEnv so helpers can read .env store.
// ---------------------------------------------------------------------------

var activeLoader *Loader

// lookup checks the active loader's .env store first, then os.LookupEnv.
// This means system env vars always win over .env file values.
func lookup(key string) (string, bool) {
	// System env wins.
	if v, ok := os.LookupEnv(key); ok {
		return v, true
	}
	// Fall back to .env store.
	if activeLoader != nil {
		if v, ok := activeLoader.envStore[key]; ok {
			return v, true
		}
	}
	return "", false
}

// ---------------------------------------------------------------------------
// Helpers — pure functions, no reflection. Use inside FromEnv().
// ---------------------------------------------------------------------------

// Str returns the env var value or the fallback.
func Str(key, fallback string) string {
	if v, ok := lookup(key); ok {
		return v
	}
	return fallback
}

// Int returns the env var parsed as int or the fallback.
func Int(key string, fallback int) int {
	if v, ok := lookup(key); ok {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return fallback
}

// Int64 returns the env var parsed as int64 or the fallback.
func Int64(key string, fallback int64) int64 {
	if v, ok := lookup(key); ok {
		if n, err := strconv.ParseInt(v, 10, 64); err == nil {
			return n
		}
	}
	return fallback
}


// Float returns the env var parsed as float64 or the fallback.
func Float(key string, fallback float64) float64 {
	if v, ok := lookup(key); ok {
		if f, err := strconv.ParseFloat(v, 64); err == nil {
			return f
		}
	}
	return fallback
}

// Bool returns the env var parsed as bool or the fallback.
// Accepts: 1, t, true, yes, on (and their negatives).
func Bool(key string, fallback bool) bool {
	if v, ok := lookup(key); ok {
		if b, err := strconv.ParseBool(v); err == nil {
			return b
		}
		switch strings.ToLower(v) {
		case "yes", "on":
			return true
		case "no", "off":
			return false
		}
	}
	return fallback
}

// Duration returns the env var parsed as time.Duration or the fallback.
// Accepts Go duration strings like "30s", "5m", "1h30m".
func Duration(key string, fallback time.Duration) time.Duration {
	if v, ok := lookup(key); ok {
		if d, err := time.ParseDuration(v); err == nil {
			return d
		}
	}
	return fallback
}

// Strings returns the env var split by sep, or the fallback slice.
// Common usage: Strings("ALLOWED_ORIGINS", ",", []string{"*"})
func Strings(key, sep string, fallback []string) []string {
	if v, ok := lookup(key); ok && v != "" {
		parts := strings.Split(v, sep)
		out := make([]string, 0, len(parts))
		for _, p := range parts {
			if t := strings.TrimSpace(p); t != "" {
				out = append(out, t)
			}
		}
		if len(out) > 0 {
			return out
		}
	}
	return fallback
}

// Uint returns the env var parsed as uint or the fallback.
func Uint(key string, fallback uint) uint {
	if v, ok := lookup(key); ok {
		if n, err := strconv.ParseUint(v, 10, 64); err == nil {
			return uint(n)
		}
	}
	return fallback
}

