package cfg

import (
	"os"
	"path/filepath"
	"testing"
	"time"
	"fmt"
)

// ---------------------------------------------------------------------------
// Test config struct — no tags, no reflection, just methods.
// ---------------------------------------------------------------------------

type testConfig struct {
	Host    string `yaml:"host"`
	Port    int    `yaml:"port"`
	Debug   bool   `yaml:"debug"`
	DBUrl   string `yaml:"db_url"`
	Timeout time.Duration
	Tags    []string
}

func (c *testConfig) Defaults() {
	c.Host = "localhost"
	c.Port = 8080
	c.Debug = false
	c.Timeout = 30 * time.Second
}

func (c *testConfig) FromEnv() {
	c.Host = Str("HOST", c.Host)
	c.Port = Int("PORT", c.Port)
	c.Debug = Bool("DEBUG", c.Debug)
	c.DBUrl = Str("DATABASE_URL", c.DBUrl)
	c.Timeout = Duration("TIMEOUT", c.Timeout)
	c.Tags = Strings("TAGS", ",", c.Tags)
}

func (c *testConfig) Validate() error {
	if c.Port < 1 || c.Port > 65535 {
		return fmt.Errorf("port must be 1-65535, got %d", c.Port)
	}
	return nil
}

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

func TestDefaults(t *testing.T) {
	var c testConfig
	err := New().NoYaml().NoEnvFile().Load(&c)
	if err != nil {
		t.Fatal(err)
	}
	if c.Host != "localhost" {
		t.Errorf("host = %q, want localhost", c.Host)
	}
	if c.Port != 8080 {
		t.Errorf("port = %d, want 8080", c.Port)
	}
	if c.Timeout != 30*time.Second {
		t.Errorf("timeout = %v, want 30s", c.Timeout)
	}
}

func TestEnvOverridesDefaults(t *testing.T) {
	t.Setenv("HOST", "0.0.0.0")
	t.Setenv("PORT", "3000")
	t.Setenv("DEBUG", "yes")
	t.Setenv("TIMEOUT", "5m")
	t.Setenv("TAGS", "api, web, worker")

	var c testConfig
	err := New().NoYaml().NoEnvFile().Load(&c)
	if err != nil {
		t.Fatal(err)
	}
	if c.Host != "0.0.0.0" {
		t.Errorf("host = %q, want 0.0.0.0", c.Host)
	}
	if c.Port != 3000 {
		t.Errorf("port = %d, want 3000", c.Port)
	}
	if !c.Debug {
		t.Error("debug = false, want true")
	}
	if c.Timeout != 5*time.Minute {
		t.Errorf("timeout = %v, want 5m", c.Timeout)
	}
	if len(c.Tags) != 3 || c.Tags[0] != "api" || c.Tags[2] != "worker" {
		t.Errorf("tags = %v, want [api web worker]", c.Tags)
	}
}

func TestYamlLoading(t *testing.T) {
	dir := t.TempDir()
	yamlContent := `host: db.example.com
port: 5432
debug: true
db_url: postgres://localhost/test
`
	os.WriteFile(filepath.Join(dir, "config.yml"), []byte(yamlContent), 0644)

	var c testConfig
	err := New().WithRoot(dir).NoEnvFile().Load(&c)
	if err != nil {
		t.Fatal(err)
	}
	if c.Host != "db.example.com" {
		t.Errorf("host = %q, want db.example.com", c.Host)
	}
	if c.Port != 5432 {
		t.Errorf("port = %d, want 5432", c.Port)
	}
	if c.DBUrl != "postgres://localhost/test" {
		t.Errorf("db_url = %q, want postgres://localhost/test", c.DBUrl)
	}
}

func TestEnvFileLoading(t *testing.T) {
	dir := t.TempDir()
	envContent := `# Database config
HOST=envfile-host
PORT=9090
DATABASE_URL="postgres://user:p@ss=word@db/myapp"
export DEBUG=true
`
	os.WriteFile(filepath.Join(dir, ".env"), []byte(envContent), 0644)

	var c testConfig
	err := New().WithRoot(dir).NoYaml().Load(&c)
	if err != nil {
		t.Fatal(err)
	}
	if c.Host != "envfile-host" {
		t.Errorf("host = %q, want envfile-host", c.Host)
	}
	if c.Port != 9090 {
		t.Errorf("port = %d, want 9090", c.Port)
	}
	// Quoted value with = inside should be handled.
	if c.DBUrl != "postgres://user:p@ss=word@db/myapp" {
		t.Errorf("db_url = %q, want postgres://user:p@ss=word@db/myapp", c.DBUrl)
	}
	if !c.Debug {
		t.Error("debug = false, want true (from export DEBUG=true)")
	}
}

func TestSystemEnvWinsOverEnvFile(t *testing.T) {
	dir := t.TempDir()
	envContent := `PORT=1111
`
	os.WriteFile(filepath.Join(dir, ".env"), []byte(envContent), 0644)

	t.Setenv("PORT", "2222")

	var c testConfig
	err := New().WithRoot(dir).NoYaml().Load(&c)
	if err != nil {
		t.Fatal(err)
	}
	if c.Port != 2222 {
		t.Errorf("port = %d, want 2222 (system env should win)", c.Port)
	}
}

func TestEnvOverridesYaml(t *testing.T) {
	dir := t.TempDir()
	yamlContent := `host: yaml-host
port: 4000
`
	os.WriteFile(filepath.Join(dir, "config.yml"), []byte(yamlContent), 0644)

	t.Setenv("PORT", "5000")

	var c testConfig
	err := New().WithRoot(dir).NoEnvFile().Load(&c)
	if err != nil {
		t.Fatal(err)
	}
	if c.Host != "yaml-host" {
		t.Errorf("host = %q, want yaml-host (from yaml)", c.Host)
	}
	if c.Port != 5000 {
		t.Errorf("port = %d, want 5000 (env should override yaml)", c.Port)
	}
}

func TestValidationFails(t *testing.T) {
	t.Setenv("PORT", "0")

	var c testConfig
	err := New().NoYaml().NoEnvFile().Load(&c)
	if err == nil {
		t.Fatal("expected validation error for port=0")
	}
}

func TestNoInterfacesImplemented(t *testing.T) {
	type bareConfig struct {
		Name string `yaml:"name"`
	}
	var c bareConfig
	err := New().NoYaml().NoEnvFile().Load(&c)
	if err != nil {
		t.Fatal(err)
	}
	if c.Name != "" {
		t.Errorf("name = %q, want empty", c.Name)
	}
}

func TestNoEnvFileSideEffects(t *testing.T) {
	dir := t.TempDir()
	envContent := `SIDE_EFFECT_TEST=leaked
`
	os.WriteFile(filepath.Join(dir, ".env"), []byte(envContent), 0644)

	var c testConfig
	New().WithRoot(dir).NoYaml().Load(&c)

	// The .env value should NOT be in os.Environ.
	if v := os.Getenv("SIDE_EFFECT_TEST"); v != "" {
		t.Errorf("SIDE_EFFECT_TEST leaked into os env: %q", v)
	}
}

func TestFullPipeline(t *testing.T) {
	dir := t.TempDir()

	yaml := `host: yaml-host
port: 3000
`
	os.WriteFile(filepath.Join(dir, "app.yml"), []byte(yaml), 0644)

	env := `DATABASE_URL=postgres://fromenvfile
DEBUG=true
`
	os.WriteFile(filepath.Join(dir, ".env.local"), []byte(env), 0644)

	t.Setenv("PORT", "9999")

	var c testConfig
	err := New().
		WithRoot(dir).
		WithYaml("app.yml").
		WithEnvFile(".env.local").
		Load(&c)

	if err != nil {
		t.Fatal(err)
	}

	// Defaults: Host=localhost, Port=8080
	// YAML overrides: Host=yaml-host, Port=3000
	// .env file: DATABASE_URL=postgres://fromenvfile, DEBUG=true
	// System env: PORT=9999 (wins over yaml 3000 AND .env file)
	// Final: Host=yaml-host, Port=9999, DBUrl=postgres://fromenvfile, Debug=true

	if c.Host != "yaml-host" {
		t.Errorf("host = %q, want yaml-host", c.Host)
	}
	if c.Port != 9999 {
		t.Errorf("port = %d, want 9999", c.Port)
	}
	if c.DBUrl != "postgres://fromenvfile" {
		t.Errorf("db_url = %q, want postgres://fromenvfile", c.DBUrl)
	}
	if !c.Debug {
		t.Error("debug should be true")
	}
	if c.Timeout != 30*time.Second {
		t.Errorf("timeout = %v, want 30s (from defaults, not overridden)", c.Timeout)
	}
}

func TestBadIntIgnored(t *testing.T) {
	t.Setenv("PORT", "not-a-number")

	var c testConfig
	err := New().NoYaml().NoEnvFile().Load(&c)
	if err != nil {
		t.Fatal(err)
	}
	if c.Port != 8080 {
		t.Errorf("port = %d, want 8080 (fallback when parse fails)", c.Port)
	}
}

func TestHelperTypes(t *testing.T) {
	t.Setenv("MY_FLOAT", "3.14")
	t.Setenv("MY_INT64", "9999999999")
	t.Setenv("MY_UINT", "42")
	t.Setenv("MY_BOOL_ON", "on")
	t.Setenv("MY_BOOL_NO", "no")

	if v := Float("MY_FLOAT", 0); v != 3.14 {
		t.Errorf("float = %v, want 3.14", v)
	}
	if v := Int64("MY_INT64", 0); v != 9999999999 {
		t.Errorf("int64 = %v, want 9999999999", v)
	}
	if v := Uint("MY_UINT", 0); v != 42 {
		t.Errorf("uint = %v, want 42", v)
	}
	if v := Bool("MY_BOOL_ON", false); !v {
		t.Error("bool 'on' should be true")
	}
	if v := Bool("MY_BOOL_NO", true); v {
		t.Error("bool 'no' should be false")
	}
}
