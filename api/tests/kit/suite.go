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

type IntegrationSuite struct {
	suite.Suite
	Env *Env
}

func (s *IntegrationSuite) SetupTest() {
	s.Env = BuildEnv(s.T())
}

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
