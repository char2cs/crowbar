//go:build integration && unix

package tests

import (
	"net/http"
	"syscall"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRegression_Symptom4_ASwitchWhileATurnRunsIsRefusedNotForced(t *testing.T) {
	c := newTurnChat(t, newHarness(t))
	writeProviderDescriptor(t, c.h, "quietstub", quietStubProviderDescriptorYAML)
	c.startTurn(t, c.launch, "A")

	_ = c.h.raw(http.MethodPost, repoBase(c.imported)+"/chats/"+c.chatID+"/switch",
		map[string]string{"provider": "quietstub"}, http.StatusConflict).Body.Close()
	require.NoError(t, syscall.Kill(c.launch.PID, 0), "the running CLI was not killed")
	assert.True(t, c.working(t))
}
