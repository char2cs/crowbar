package chat

import (
	"context"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/adapter/store"
	ledgerstore "github.com/char2cs/crowbar/api/internal/adapter/store/ledger"
	storesqlite "github.com/char2cs/crowbar/api/internal/adapter/store/sqlite"
	agentchat "github.com/char2cs/crowbar/api/internal/app/repositories/chat"
	agenttools "github.com/char2cs/crowbar/api/internal/app/usecases/chat/internal/shared/tools"
	"github.com/char2cs/crowbar/api/internal/app/usecases/mocks"
	engineterminal "github.com/char2cs/crowbar/api/internal/core/terminal"
	"github.com/char2cs/crowbar/api/internal/domain"
	engineagents "github.com/char2cs/crowbar/api/internal/engine/agents"
)

type wiringChats struct{ agentchat.EventStore }

type wiringWorkspace struct{ WorkspaceReader }

type wiringWorktree struct{ WorktreeCreator }

type wiringLineage struct{ ChatLineage }

type wiringPrefs struct {
	store.Store[domain.AgentProviderPreference, string]
}

type screenReadingCommander struct{}

func (screenReadingCommander) CreateCommand(
	context.Context, string, string, []string, []string, func(engineterminal.Exit),
) (string, error) {
	return "", nil
}

func (screenReadingCommander) TerminateGraceful(context.Context, string) error { return nil }

func (screenReadingCommander) Write(context.Context, string, []byte) error { return nil }

func (screenReadingCommander) SessionLive(context.Context, string) bool { return false }

func (screenReadingCommander) Screen(string, uint64) (string, uint64, bool) {
	return "", 0, false
}

var constructorUnsetFields = map[string]string{}

var sharedInstances = map[string][]string{
	"the chat locks":      {"ingress.locks", "runners.chatLock"},
	"the telemetry store": {"conversations.telemetry", "recorder.telemetry"},
	"the answer desk":     {"answers", "recorder.answers", "runners.answers"},
	"the chat record":     {"conversations", "recorder.titles", "runners.conversations"},
	"the provider table":  {"providers", "runners.providers"},
	"the CLI lifecycle":   {"runners", "conversations.runners", "recorder.runners", "ingress.runners"},
	"the turn record":     {"recorder", "ingress.turns", "ingress.descriptors"},
	"the chat ledger":     {"ledger", "conversations.ledger", "recorder.ledger", "runners.ledger", "ingress.ledger"},
	"the payload store":   {"blobs", "conversations.blobs", "recorder.blobs"},
}

var nilableKinds = map[reflect.Kind]bool{
	reflect.Pointer:   true,
	reflect.Map:       true,
	reflect.Func:      true,
	reflect.Interface: true,
	reflect.Chan:      true,
	reflect.Slice:     true,
}

func newWiringFixture(t *testing.T) *Usecase {
	t.Helper()
	minter, err := agenttools.NewTokenMinter()
	require.NoError(t, err)
	home := t.TempDir()
	agentsEngine := engineagents.New()
	t.Cleanup(agentsEngine.Close)
	db, err := storesqlite.OpenDB(":memory:")
	require.NoError(t, err)
	chatLedger, err := ledgerstore.Open(t.Context(), db, time.Now)
	require.NoError(t, err)
	blobs, err := ledgerstore.OpenBlobs(t.TempDir())
	require.NoError(t, err)
	return New(Deps{
		Chats:         wiringChats{},
		Blobs:         blobs,
		Agents:        agentsEngine,
		Terminal:      screenReadingCommander{},
		Workspace:     wiringWorkspace{},
		Worktree:      wiringWorktree{},
		Lineage:       wiringLineage{},
		ProviderPrefs: wiringPrefs{},
		Home:          func() (string, error) { return home, nil },
		Ledger:        chatLedger,
		Minter:        minter,
		Tools:         agenttools.Deps{},
		Folders:       mocks.NewFolderStore(),
		Nodes:         mocks.NewNodePlacements(),
		RepoRoots:     mocks.NewAgentWorkspaceGitStatus(),
	})
}

func TestNew_LeavesNoConstructorOwnedFieldNil(t *testing.T) {
	u := newWiringFixture(t)

	for prefix, value := range walkable(u) {
		fields := reflect.ValueOf(value).Elem()
		require.NotZerof(t, fields.NumField(), "%s has no fields to guard", prefix)
		for i := range fields.NumField() {
			assertFieldWired(t, qualify(prefix, fields.Type().Field(i).Name), fields.Field(i))
		}
	}
	assertExcludedFieldsStillExist(t, u)
}

func walkable(u *Usecase) map[string]any {
	return map[string]any{
		"":              u,
		"conversations": u.conversations,
		"recorder":      u.recorder,
		"runners":       u.runners,
		"providers":     u.providers,
		"ingress":       u.ingress,
	}
}

func qualify(prefix, field string) string {
	if prefix == "" {
		return field
	}
	return prefix + "." + field
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
		"New left %s nil. Nothing else fails until the daemon dereferences it at "+
			"runtime, so either restore the initialiser or add %s to "+
			"constructorUnsetFields with the reason it is legitimately nil.",
		name, name)
}

func assertExcludedFieldsStillExist(t *testing.T, u *Usecase) {
	t.Helper()
	owners := map[string]reflect.Type{}
	for prefix, value := range walkable(u) {
		owners[prefix] = reflect.TypeOf(value).Elem()
	}
	for name, reason := range constructorUnsetFields {
		prefix, field, _ := strings.Cut(name, ".")
		if field == "" {
			prefix, field = "", prefix
		}
		owner, known := owners[prefix]
		if !known {
			assert.Failf(t, "unknown component",
				"constructorUnsetFields excuses %q (%s) but %q is not one of the "+
					"components New builds", name, reason, prefix)
			continue
		}
		_, ok := owner.FieldByName(field)
		assert.Truef(t, ok,
			"constructorUnsetFields excuses %q (%s) but no such field exists",
			name, reason)
	}
}

func TestNew_SharesOneInstanceOfEachPieceOfInFlightState(t *testing.T) {
	u := newWiringFixture(t)

	for what, fields := range sharedInstances {
		require.Greater(t, len(fields), 1, "%s: a sharing claim needs at least two holders", what)
		first := fieldPointer(t, u, fields[0])
		for _, field := range fields[1:] {
			assert.Equalf(t, first, fieldPointer(t, u, field),
				"%s is not shared: %s and %s are different instances", what, fields[0], field)
		}
	}
}

func fieldPointer(t *testing.T, u *Usecase, path string) uintptr {
	t.Helper()
	prefix, field, _ := strings.Cut(path, ".")
	if field == "" {
		prefix, field = "", prefix
	}
	owner, known := walkable(u)[prefix]
	require.Truef(t, known, "%q names no component New builds", path)
	value := reflect.ValueOf(owner).Elem().FieldByName(field)
	require.Truef(t, value.IsValid(), "%q does not exist", path)
	require.Falsef(t, value.IsNil(), "%q is nil", path)
	if value.Kind() == reflect.Interface {
		value = value.Elem()
	}
	require.Equalf(t, reflect.Pointer, value.Kind(), "%q is not a pointer", path)
	return value.Pointer()
}

func TestNew_WiresTheToolSurfaceBackToTheUsecase(t *testing.T) {
	u := newWiringFixture(t)

	assert.Same(t, u, u.tools.Chats,
		"New must self-assign the Chats port; a caller cannot, because the "+
			"usecase does not exist when it builds the Deps")
	assert.Same(t, u, u.tools.ChatLogs,
		"New must self-assign the ChatLogs port; without it get_chat_log is not "+
			"registered at all")
	assert.Same(t, u, u.tools.Lineage,
		"New must self-assign the Lineage port; without it get_chat_log cannot "+
			"tell a thread from a sibling and is withdrawn entirely")
	require.NotNil(t, u.tools.ToolAccess,
		"New must wire ToolAccess to providerMCPEnabled. A nil ToolAccess FAILS "+
			"OPEN — Deps.refuseDisabledTools returns nil early, so every registered "+
			"tool stays callable and the per-provider Tools switch the user turned "+
			"OFF in Settings is silently back on")
	assert.Equal(t,
		reflect.ValueOf(u.providerMCPEnabled).Pointer(),
		reflect.ValueOf(u.tools.ToolAccess).Pointer(),
		"and it must be providerMCPEnabled itself: a non-nil port bound to "+
			"something else reads the wrong preference and the switch still does "+
			"not hold")
}

func TestInvariant_TheOnlyWayToEndARunnerIsGraceful(t *testing.T) {
	t.Parallel()
	var _ TerminalCommander = terminalCommanderShape{}
}

type terminalCommanderShape struct{}

func (terminalCommanderShape) CreateCommand(
	_ context.Context, _, _ string, _, _ []string, _ func(engineterminal.Exit),
) (string, error) {
	return "", nil
}

func (terminalCommanderShape) TerminateGraceful(_ context.Context, _ string) error {
	return nil
}

func (terminalCommanderShape) Write(_ context.Context, _ string, _ []byte) error { return nil }

func (terminalCommanderShape) SessionLive(_ context.Context, _ string) bool { return false }
