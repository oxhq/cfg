# cfg

> Zero-reflection config loader for Go.

Loads configuration through explicit Go methods — no reflection, no struct tags, no codegen, no magic.

## Install

```bash
go get github.com/oxhq/cfg
```

## Usage

Define your config struct. Implement whichever interfaces you need:

```go
package main

import (
    "fmt"
    "log"
    "time"

    "github.com/oxhq/cfg"
)

type Config struct {
    Host    string        `yaml:"host"`
    Port    int           `yaml:"port"`
    Debug   bool          `yaml:"debug"`
    DBUrl   string        `yaml:"db_url"`
    Timeout time.Duration
    Origins []string
}

func (c *Config) Defaults() {
    c.Host = "localhost"
    c.Port = 8080
    c.Timeout = 30 * time.Second
    c.Origins = []string{"*"}
}

func (c *Config) FromEnv() {
    c.Host    = cfg.Str("HOST", c.Host)
    c.Port    = cfg.Int("PORT", c.Port)
    c.Debug   = cfg.Bool("DEBUG", c.Debug)
    c.DBUrl   = cfg.Str("DATABASE_URL", c.DBUrl)
    c.Timeout = cfg.Duration("TIMEOUT", c.Timeout)
    c.Origins = cfg.Strings("ALLOWED_ORIGINS", ",", c.Origins)
}

func (c *Config) Validate() error {
    if c.Port < 1 || c.Port > 65535 {
        return fmt.Errorf("port must be 1-65535, got %d", c.Port)
    }
    if c.DBUrl == "" {
        return fmt.Errorf("DATABASE_URL is required")
    }
    return nil
}

func main() {
    var c Config
    err := cfg.New().
        WithYaml("config.yml").
        WithEnvFile(".env").
        Load(&c)
    if err != nil {
        log.Fatal(err)
    }
    fmt.Printf("Listening on %s:%d\n", c.Host, c.Port)
}
```

## Pipeline

```
Defaults() → YAML file → .env file → system env vars → FromEnv() → Validate()
```

Each layer overrides the previous. System environment variables always win.

## Interfaces

All are optional. Implement what you need:

| Interface | Method | Purpose |
|-----------|--------|---------|
| `Defaulter` | `Defaults()` | Set zero-config defaults |
| `EnvLoader` | `FromEnv()` | Map env vars to fields |
| `Validator` | `Validate() error` | Check final config |

## Helpers

Use inside `FromEnv()`:

| Helper | Signature | Example |
|--------|-----------|---------|
| `Str` | `Str(key, fallback string) string` | `cfg.Str("HOST", "localhost")` |
| `Int` | `Int(key string, fallback int) int` | `cfg.Int("PORT", 8080)` |
| `Int64` | `Int64(key string, fallback int64) int64` | `cfg.Int64("MAX_SIZE", 0)` |
| `Uint` | `Uint(key string, fallback uint) uint` | `cfg.Uint("WORKERS", 4)` |
| `Float` | `Float(key string, fallback float64) float64` | `cfg.Float("RATE", 1.0)` |
| `Bool` | `Bool(key string, fallback bool) bool` | `cfg.Bool("DEBUG", false)` |
| `Duration` | `Duration(key string, fallback time.Duration) time.Duration` | `cfg.Duration("TIMEOUT", 30*time.Second)` |
| `Strings` | `Strings(key, sep string, fallback []string) []string` | `cfg.Strings("TAGS", ",", nil)` |

`Bool` accepts: `1/0`, `true/false`, `t/f`, `yes/no`, `on/off`.

## .env file

The parser handles:
- Comments (`# ...`)
- Quoted values (`"value"` or `'value'`)
- `export` prefix (`export VAR=val`)
- Values containing `=` (`PASSWORD="abc=123"`)
- No `os.Setenv` side effects (isolated store)

## Design

- **No reflection** — your `FromEnv()` method is plain Go code
- **No custom struct tags** — only standard `yaml` tags for YAML unmarshaling
- **No codegen** — nothing generated, nothing hidden
- **No side effects** — `.env` values stay in an isolated store
- **One file, one dep** — `gopkg.in/yaml.v3`

## License

MIT
