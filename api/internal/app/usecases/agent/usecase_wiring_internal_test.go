package agent

import (
	"reflect"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/adapter/store"
	"github.com/char2cs/crowbar/api/internal/app/repositories/agentactivity"
	"github.com/char2cs/crowbar/api/internal/app/repositories/agentchat"
	"github.com/char2cs/crowbar/api/internal/app/repositories/agentrunner"
	"github.com/char2cs/crowbar/api/internal/app/usecases/agenttools"
	"github.com/char2cs/crowbar/api/internal/domain"
	engineagents "github.com/char2cs/crowbar/api/internal/engine/agents"
)

type wiringChats struct{ agentchat.EventStore }

type wiringRunners struct{ agentrunner.EventStore }

type wiringActivity struct{ agentactivity.EventStore }

type wiringWorkspace struct{ WorkspaceReader }

type wiringLineage struct{ ChatLineage }

type wiringPrefs struct {
	store.Store[domain.AgentProviderPreference, string]
}

// constructorUnsetFields are the Usecase fields agent.New is deliberately NOT
// responsible for. Every other nilable field must come back non-nil, so a field
// added to the struct is guarded the day it is added; anything added here has to
// carry the reason it is legitimately nil after construction.
var constructorUnsetFields = map[string]string{
	"promptSettled": "wired by StartTerminalWaitSweep, not New: it publishes " +
		"through the hub, a layer above this one, and stays nil in a daemon with " +
		"no detector (usecase.go).",
	"messageDelta": "wired by StartTerminalWaitSweep beside promptSettled, for " +
		"the same reason: a daemon with nobody to publish to records the message " +
		"when it finishes instead (usecase.go).",
}

var nilableKinds = map[reflect.Kind]bool{
	reflect.Pointer:   true,
	reflect.Map:       true,
	reflect.Func:      true,
	reflect.Interface: true,
	reflect.Chan:      true,
	reflect.Slice:     true,
}

// newWiringFixture builds a Usecase through the REAL agent.New, over the
// cheapest dependency that is still the production SHAPE of each port.
//
// Two choices are load-bearing. The commander is screenReadingCommander because
// it implements termwait.Screens, which the production terminal engine also does
// (see the compile-time assertion in terminal_wait_internal_test.go) — the
// external harness's fakeCommander does not, so a fixture built on that one
// leaves termWait nil and would force this test to excuse the constructor's last
// initialiser. And installed is passed as nil, exactly as the container does, so
// New's default install probe is part of what is being checked.
func newWiringFixture(t *testing.T) *Usecase {
	t.Helper()
	minter, err := agenttools.NewTokenMinter()
	require.NoError(t, err)
	home := t.TempDir()
	return New(
		wiringChats{},
		wiringRunners{},
		wiringActivity{},
		engineagents.New(),
		screenReadingCommander{},
		wiringWorkspace{},
		wiringLineage{},
		wiringPrefs{},
		func() (string, error) { return home, nil },
		nil,
		minter,
		agenttools.Deps{},
	)
}

func TestNew_LeavesNoConstructorOwnedFieldNil(t *testing.T) {
	u := newWiringFixture(t)
	require.NotNil(t, u.termWait,
		"fixture precondition: the commander renders screens, so termWait is "+
			"under test here rather than excused")

	fields := reflect.ValueOf(u).Elem()
	require.NotZero(t, fields.NumField())
	for i := range fields.NumField() {
		assertFieldWired(t, fields.Type().Field(i).Name, fields.Field(i))
	}
	assertExcludedFieldsStillExist(t, fields.Type())
}

func assertFieldWired(
	t *testing.T,
	name string,
	field reflect.Value,
) {
	t.Helper()
	if _, excluded := constructorUnsetFields[name]; excluded {
		return
	}
	if !nilableKinds[field.Kind()] {
		return
	}
	assert.Falsef(t, field.IsNil(),
		"agent.New left Usecase.%s nil. Nothing else fails until the daemon "+
			"dereferences it at runtime, so either restore the initialiser or add "+
			"%s to constructorUnsetFields with the reason it is legitimately nil.",
		name, name)
}

func assertExcludedFieldsStillExist(
	t *testing.T,
	usecase reflect.Type,
) {
	t.Helper()
	for name, reason := range constructorUnsetFields {
		_, ok := usecase.FieldByName(name)
		assert.Truef(t, ok,
			"constructorUnsetFields excuses %q (%s) but no such field exists — a "+
				"stale exclusion silently stops guarding whatever replaced it",
			name, reason)
	}
}

func TestNew_WiresTheToolSurfaceBackToTheUsecase(t *testing.T) {
	u := newWiringFixture(t)

	assert.Same(t, u, u.tools.Chats,
		"New must self-assign the Chats port; a caller cannot, because u does not "+
			"exist when it builds the Deps")
	assert.Same(t, u, u.tools.ChatLogs,
		"New must self-assign the ChatLogs port; without it get_chat_log is not "+
			"registered at all")
	assert.NotNil(t, u.tools.ToolAccess,
		"New must wire ToolAccess to providerMCPEnabled. A nil ToolAccess FAILS "+
			"OPEN — Deps.refuseDisabledTools returns nil early, so every registered "+
			"tool stays callable and the per-provider Tools switch the user turned "+
			"OFF in Settings is silently back on")
}
