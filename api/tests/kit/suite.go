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
	removeFakeCLI()
	if cleanup != "" {
		if err := os.RemoveAll(cleanup); err != nil {
			fmt.Fprintf(os.Stderr, "kit: remove the isolated crowbar home: %v\n", err)
		}
	}
	return code
}

// isolateHome points the binary at a throwaway Crowbar home. A CROWBAR_HOME
// naming a non-default home inside os.TempDir() is honoured; anything else,
// unset included, is silently replaced. It returns the directory to remove,
// or "" if none.
func isolateHome() string {
	if h, err := home.Resolve(); err == nil && !h.IsDefault() {
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
