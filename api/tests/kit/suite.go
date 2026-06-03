//go:build integration || manual

// Package kit provides the shared test harness for all Crowbar integration suites.
package kit

import "github.com/stretchr/testify/suite"

// IntegrationSuite is the shared base embedded by all integration suite types.
// Each test gets a fresh isolated Env via SetupTest; the Env is torn down in
// TeardownTest so no state leaks between tests.
type IntegrationSuite struct {
	suite.Suite
	Env *Env
}

// SetupTest creates a fresh isolated environment for each test.
func (s *IntegrationSuite) SetupTest() {
	s.Env = NewEnv(s.T())
}

// TeardownTest shuts down the in-process server and cleans up the temp directory.
func (s *IntegrationSuite) TeardownTest() {
	s.Env.Close()
}
