# Agent 02 — Core Utilities

**Working directory:** `/Users/char2cs/Projects/Rabbyte/crowbar/api`
**Module:** `github.com/char2cs/crowbar/api`

## Context

The `core/` layer provides path resolution, configuration loading, and metadata — shared by every other layer. It has no dependencies on domain, app, or API packages.

## Files to read before starting

- `docs/superpowers/specs/2026-05-19-domain-crud-design.md` §1 (core utilities)
- `api/ARCHITECTURE.md` §"Core Utilities"
- Quiver reference: `/Users/char2cs/Projects/Rabbyte/quiver.core/internal/core/` — read all files for implementation patterns

## What already exists

Agent 01 fixed import paths. The `internal/core/gateway/` package may already exist from the scaffold — leave it untouched.

## Tasks

### `internal/core/metadata/`

Embed a `metadata.yaml` file that contains the application name, version, and default home directory path template.

```yaml
name: crowbar
version: v0
home: "{{home}}/.crowbar"
```

`metadata.go`: reads the embedded YAML; exposes `GetHome() string` which resolves `{{home}}` to the OS home directory. For cross-platform support, use build tags:
- `resolve_home.go` (non-Windows): uses `os.UserHomeDir()`
- `resolve_home_windows.go`: uses `os.UserHomeDir()` with `//go:build windows`

Expose:
```go
func GetHome() string                        // resolved ~/.crowbar
func GetHomeAt(dir string) string            // override for tests
```

### `internal/core/config/`

Loads `~/.crowbar/config.yaml` over embedded defaults. Singleton via `sync.Once`.

Embedded default `config.yaml`:
```yaml
intelligence:
  low: claude-haiku-4-5-20251001
  medium: claude-sonnet-4-6
  high: claude-opus-4-7
api:
  host: "unix:///{{home}}/.crowbar/state/crowbar.sock"
```

Expose:
```go
type IntelligenceConfig struct {
    Low    string `yaml:"low"`
    Medium string `yaml:"medium"`
    High   string `yaml:"high"`
}

type APIConfig struct {
    Host string `yaml:"host"`
}

type Config struct {
    Intelligence IntelligenceConfig `yaml:"intelligence"`
    API          APIConfig          `yaml:"api"`
}

func Get() Config             // singleton; loads once
func GetIntelligence() IntelligenceConfig
func GetAPI() APIConfig
func (c IntelligenceConfig) ModelFor(level string) string
```

`ModelFor` maps `"low"/"medium"/"high"` to the corresponding model name string. Returns empty string for unknown levels.

### `internal/core/paths/`

Provides lazy-mkdir path helpers. Every path is derived from the home directory. Per-path `sync.Mutex` ensures concurrent callers don't race on `os.MkdirAll`.

```go
// Using process home dir
func Events() (string, error)   // ~/.crowbar/state/events/
func Store() (string, error)    // ~/.crowbar/state/store/
func Runs() (string, error)     // ~/.crowbar/runs/
func Logs() (string, error)     // ~/.crowbar/logs/

// Using explicit home dir (for test isolation)
func EventsAt(homeDir string) (string, error)
func StoreAt(homeDir string) (string, error)
func RunsAt(homeDir string) (string, error)
func LogsAt(homeDir string) (string, error)
```

Each function calls `os.MkdirAll(path, 0o755)` before returning. Use a `sync.Mutex` per logical path to avoid TOCTOU races in concurrent test environments.

## Verification

```bash
cd /Users/char2cs/Projects/Rabbyte/crowbar/api && go build ./internal/core/...
go vet ./internal/core/...
```
