//go:build integration

package kit

import (
	"fmt"
	"log/slog"
	"os"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/suite"

	"github.com/char2cs/crowbar/api/internal/core/home"
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
	os.Exit(run(m))
}

// MainGuardingProviderHomes is Main for a package that spawns REAL vendor CLIs. It
// additionally brackets the whole run with a snapshot of the user's provider homes and
// fails the run if it added a place to one of them.
//
// It belongs at TestMain and not in a test of its own because the thing being policed
// is the WHOLE package: any test that spawns a CLI without kit.IsolateProviderHomes
// leaks, and a per-test assertion would only ever cover the tests someone remembered
// to add it to. Bracketing m.Run covers the ones written next year too.
//
// A leak fails a run that otherwise passed. That is the point — the pollution it
// catches is invisible in test output and permanent on disk, so the only moment it can
// still be cheap to fix is the run that introduced it.
func MainGuardingProviderHomes(
	m *testing.M,
) {
	before := SnapshotProviderHomes()
	code := run(m)
	added := SnapshotProviderHomes().Added(before)
	if added == "" {
		os.Exit(code)
	}
	fmt.Fprintf(os.Stderr, "\nPROVIDER HOME POLLUTION: %s\n", added)
	if code == 0 {
		code = 1
	}
	os.Exit(code)
}

func run(
	m *testing.M,
) int {
	handler := slog.NewTextHandler(
		os.Stderr,
		&slog.HandlerOptions{Level: slog.LevelError},
	)
	slog.SetDefault(slog.New(handler))
	gin.SetMode(gin.TestMode)

	cleanup := isolateHome()

	code := m.Run()
	if cleanup != "" {
		_ = os.RemoveAll(cleanup)
	}
	return code
}

// isolateHome points the whole binary at a throwaway Crowbar home, so nothing
// that resolves a home without an injected one reaches the developer's real
// ~/.crowbar. An explicit CROWBAR_HOME is honoured as-is, though home.Resolve
// still refuses one outside os.TempDir() in a test binary. It returns the
// directory to remove, or "" when it made none.
func isolateHome() string {
	if os.Getenv(home.EnvVar) != "" {
		return ""
	}
	isolated, err := os.MkdirTemp("", "crowbar-test-home")
	if err != nil {
		panic("kit: create isolated crowbar home: " + err.Error())
	}
	if setErr := os.Setenv(home.EnvVar, isolated); setErr != nil {
		panic("kit: set " + home.EnvVar + ": " + setErr.Error())
	}
	return isolated
}
