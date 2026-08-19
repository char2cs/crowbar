//go:build integration

package kit

import (
	"log/slog"
	"os"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/suite"

	"github.com/char2cs/crowbar/api/internal/core/metadata"
)

// IntegrationSuite is the base testify suite for all integration packages.
// It wires a fresh Env (real HTTP+WS server backed by real SQLite stores)
// before every test and makes it available via the Env field.
type IntegrationSuite struct {
	suite.Suite
	// Env is the fully-wired test environment populated by SetupTest.
	Env *Env
}

// SetupTest spins up a fresh, isolated Env before each test in the suite.
// Embedding suites do not need to call BuildEnv themselves.
func (s *IntegrationSuite) SetupTest() {
	s.Env = BuildEnv(s.T())
}

// Main is the integration test harness entry point. It silences logs,
// enables gin test mode, then delegates to m.Run.
func Main(
	m *testing.M,
) {
	handler := slog.NewTextHandler(
		os.Stderr,
		&slog.HandlerOptions{Level: slog.LevelError},
	)
	slog.SetDefault(slog.New(handler))
	gin.SetMode(gin.TestMode)

	// ISOLATE THE HOME FOR THE WHOLE BINARY. Anything that resolves the crowbar
	// home without an injected one falls back to the developer's real ~/.crowbar
	// and writes there, beside real work: a live home had 110 empty project
	// directories, one per run of this suite, plus a projects/p2 from the unit
	// tests' own fixture id. An explicit CROWBAR_HOME is honoured as-is so a
	// caller can still point a run somewhere deliberate.
	cleanup := ""
	if os.Getenv(metadata.HomeEnvVar) == "" {
		home, err := os.MkdirTemp("", "crowbar-test-home")
		if err != nil {
			panic("kit: create isolated crowbar home: " + err.Error())
		}
		if setErr := os.Setenv(metadata.HomeEnvVar, home); setErr != nil {
			panic("kit: set " + metadata.HomeEnvVar + ": " + setErr.Error())
		}
		cleanup = home
	}

	code := m.Run()
	if cleanup != "" {
		_ = os.RemoveAll(cleanup)
	}
	os.Exit(code)
}
