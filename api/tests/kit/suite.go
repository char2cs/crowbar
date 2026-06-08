//go:build integration

package kit

import (
	"log/slog"
	"os"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/suite"
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
	os.Exit(m.Run())
}
