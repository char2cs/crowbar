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
	agentactivity "github.com/char2cs/crowbar/api/internal/app/repositories/chat/activity"
	agenttools "github.com/char2cs/crowbar/api/internal/app/usecases/chat/internal/shared/tools"
	"github.com/char2cs/crowbar/api/internal/app/usecases/mocks"
	engineterminal "github.com/char2cs/crowbar/api/internal/core/terminal"
	"github.com/char2cs/crowbar/api/internal/domain"
	engineagents "github.com/char2cs/crowbar/api/internal/engine/agents"
)

// ─── from wiring_internal_test.go ─────────────────────────────────────

type wiringChats struct{ agentchat.EventStore }

type wiringActivity struct{ agentactivity.EventStore }

type wiringWorkspace struct{ WorkspaceReader }

type wiringWorktree struct{ WorktreeCreator }

type wiringLineage struct{ ChatLineage }

type wiringPrefs struct {
	store.Store[domain.AgentProviderPreference, string]
}

// screenReadingCommander is a PTY seam that can render a screen, which is the
// production terminal engine's shape.
type screenReadingCommander struct{}

func (screenReadingCommander) CreateCommand(
	context.Context, string, string, []string, []string, func(engineterminal.Exit),
) (string, error) {
	return "", nil
}

func (screenReadingCommander) TerminateGraceful(context.Context, string) error { return nil }

func (screenReadingCommander) SessionLive(context.Context, string) bool { return false }

func (screenReadingCommander) Screen(string, uint64) (string, uint64, bool) {
	return "", 0, false
}

// constructorUnsetFields are the fields New is deliberately NOT responsible for,
// keyed the way assertFieldWired names them: bare for a field of Usecase itself,
// "component.field" for a field of one of the four it builds. Every other nilable
// field must come back non-nil, so a field added to any of them is guarded the day
// it is added; anything added here has to carry the reason it is legitimately nil
// after construction.
var constructorUnsetFields = map[string]string{
	"turns.feed": "wired by StartTerminalWaitSweep, not New: it publishes through " +
		"the hub, a layer above this one, and a daemon with nobody to publish to " +
		"records the message when it finishes instead (internal/turn/turns.go).",
}

// sharedInstances are the pieces of in-flight state that MUST be one instance
// across the components holding them, listed as the field each component knows
// them by, so the sharing cannot be inferred from the field name and has to be
// stated.
//
// A second instance of any of these does not fail to compile and does not fail a
// test that exercises one path at a time. It puts two CLIs on one chat.
var sharedInstances = map[string][]string{
	// A hook and a user-initiated spawn on one chat are ordered by this one lock;
	// a second would let a hook land inside the decision it contradicts.
	"the chat locks":      {"ingress.locks", "runners.chatLock"},
	"the telemetry store": {"conversations.telemetry", "turns.telemetry"},
	"the answer desk":     {"answers", "turns.answers", "runners.answers"},
	"the chat record":     {"conversations", "turns.conversations", "runners.conversations"},
	"the provider table":  {"providers", "runners.providers"},
	"the CLI lifecycle":   {"runners", "conversations.runners", "turns.runners", "ingress.runners"},
	"the turn record":     {"turns", "ingress.turns", "ingress.descriptors"},
	"the chat ledger":     {"ledger", "conversations.ledger", "turns.ledger", "runners.ledger", "ingress.ledger"},
}

var nilableKinds = map[reflect.Kind]bool{
	reflect.Pointer:   true,
	reflect.Map:       true,
	reflect.Func:      true,
	reflect.Interface: true,
	reflect.Chan:      true,
	reflect.Slice:     true,
}

// newWiringFixture builds the usecase through the REAL New, over the cheapest
// dependency that is still the production SHAPE of each port.
//
// Two choices are load-bearing. The commander renders screens, which the
// production terminal engine also does, so the CLI lifecycle this fixture builds
// gets its terminal-wait detector rather than the degraded no-detector shape.
// And installed is passed as nil, exactly as the container does, so New's default
// install probe is part of what is being checked.
func newWiringFixture(t *testing.T) *Usecase {
	t.Helper()
	minter, err := agenttools.NewTokenMinter()
	require.NoError(t, err)
	home := t.TempDir()
	agentsEngine := engineagents.New()
	t.Cleanup(agentsEngine.Close)
	db, err := storesqlite.OpenDB(":memory:")
	require.NoError(t, err)
	chatLedger, err := ledgerstore.Open(db, time.Now)
	require.NoError(t, err)
	return New(Deps{
		Chats:         wiringChats{},
		Activity:      wiringActivity{},
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
		// Folders/Nodes are ALWAYS wired in production (container.go, both
		// unconditional) — mirrored here rather than left nil, exactly like
		// every other port this fixture wires (see Installed's own doc for
		// the one deliberate exception).
		Folders:   mocks.NewFolderStore(),
		Nodes:     mocks.NewNodePlacements(),
		RepoRoots: mocks.NewAgentWorkspaceGitStatus(),
	})
}

// TestNew_LeavesNoConstructorOwnedFieldNil is the whole reason the shared state
// is built in one place: none of it fails to compile when it is forgotten, and
// none of it fails a test that exercises one path at a time. It wedges a switch
// or doubles a CLI in production instead.
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

// walkable is the usecase and every component it builds, keyed by the name a
// field of that component is qualified with.
//
// The components are walked as well as the face, and that is the point: moving a
// port down into one of them would otherwise move it out of this guard at the
// same time — which is exactly what a decomposition does, one field at a time.
func walkable(u *Usecase) map[string]any {
	return map[string]any{
		"":              u,
		"conversations": u.conversations,
		"turns":         u.turns,
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

// assertExcludedFieldsStillExist stops an exclusion outliving its field. A stale
// entry silently stops guarding whatever took the name's place.
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

// TestNew_SharesOneInstanceOfEachPieceOfInFlightState is the other half of the
// constructor's contract. The nil check above proves each piece was BUILT; this
// proves each was built ONCE and handed round, which is the part that wedges a
// daemon when it is wrong.
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

// fieldPointer reads one "component.field" (or bare "field" of Usecase) as a raw
// pointer. It reflects rather than reading the field, because the fields are
// unexported: Pointer() is readable where Interface() would panic.
//
// A sibling port is held as an INTERFACE (each component declares its own), so an
// interface field is unwrapped to the pointer inside it — otherwise two holders of
// the same component would compare unequal for having named its type differently.
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

// ─── from invariants_internal_test.go ─────────────────────────────────

// The load-bearing invariants of the in-flight tier (design spec §7).
//
// Every one of these was discovered by a production failure, and every one is
// invisible to the compiler: inverting any of them leaves code that builds, passes
// the rest of the suite, and breaks a live chat. They are gathered in one file so a
// change to the machinery has one obvious place to check.
//
// Each test was verified to FAIL when its invariant was inverted; the inversion and
// the message it produces are recorded in the comment above it.

// §7.4 — a runner is ended with SIGTERM, never SIGKILL.
//
// Graceful matters twice: a well-behaved CLI flushes its native transcript on SIGTERM,
// so neither the provider being switched out nor a runner EVICTED from a conversation
// loses its last turn — and an evicted runner's conversation is about to be read by
// the runner taking it over.
//
// The port declares exactly one way to end a CLI. Inverted (add a Kill/Terminate
// method to TerminalCommander): this stops compiling, which is the guarantee.
func TestInvariant_TheOnlyWayToEndARunnerIsGraceful(t *testing.T) {
	t.Parallel()
	// TerminalCommander must expose no forceful kill. Declaring the full interface
	// here means adding one to the port breaks this file.
	var _ TerminalCommander = terminalCommanderShape{}
}

// terminalCommanderShape is the complete set of ways this package may act on a PTY.
// It compiles only while TerminalCommander has exactly these three methods, so a
// forceful kill cannot be added to the port without this file objecting.
type terminalCommanderShape struct{}

func (terminalCommanderShape) CreateCommand(
	_ context.Context, _, _ string, _, _ []string, _ func(engineterminal.Exit),
) (string, error) {
	return "", nil
}

func (terminalCommanderShape) TerminateGraceful(_ context.Context, _ string) error {
	return nil
}

func (terminalCommanderShape) SessionLive(_ context.Context, _ string) bool { return false }
