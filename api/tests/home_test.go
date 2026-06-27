//go:build integration

package tests

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// TestRegression_HomeEndpointReachable proves that GET /v0/projects/:id/home
// returns 200 with a workspace DTO whose kind is "home". This guards the router
// wiring added in Task 5: if the home package is not mounted under projectScoped
// the endpoint 404s and the project home panel never loads.
func TestRegression_HomeEndpointReachable(t *testing.T) {
	h := newHarness(t)

	// Create a project (which auto-provisions the home workspace in Task 3).
	imported := importProject(t, h)

	// GET /home must return 200 with a home workspace DTO.
	var homeWS struct {
		Kind string `json:"kind"`
	}
	h.get("/v0/projects/"+imported.projectID+"/home", &homeWS)
	require.Equal(t, "home", homeWS.Kind,
		"home endpoint must return a workspace with kind=home")
}
