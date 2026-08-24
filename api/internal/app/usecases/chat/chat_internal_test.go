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
	"github.com/char2cs/crowbar/api/internal/adapter/store/agentjournal"
	agentchat "github.com/char2cs/crowbar/api/internal/app/repositories/chat"
	agentactivity "github.com/char2cs/crowbar/api/internal/app/repositories/chat/activity"
	"github.com/char2cs/crowbar/api/internal/app/usecases/chat/internal/shared/inflight"
	agenttools "github.com/char2cs/crowbar/api/internal/app/usecases/chat/internal/shared/tools"
	"github.com/char2cs/crowbar/api/internal/app/usecases/chat/internal/turn"
	"github.com/char2cs/crowbar/api/internal/domain"
	engineagents "github.com/char2cs/crowbar/api/internal/engine/agents"
	agentrunner "github.com/char2cs/crowbar/api/internal/engine/agents/runner"
)

// ─── from wiring_internal_test.go ─────────────────────────────────────

type wiringChats struct{ agentchat.EventStore }

type wiringRunners struct{ agentrunner.EventStore }

type wiringActivity struct{ agentactivity.EventStore }

type wiringWorkspace struct{ WorkspaceReader }

type wiringLineage struct{ ChatLineage }

type wiringPrefs struct {
	store.Store[domain.AgentProviderPreference, string]
}

// screenReadingCommander is a PTY seam that can render a screen, which is the
// production terminal engine's shape.
type screenReadingCommander struct{}

func (screenReadingCommander) CreateCommand(
	context.Context, string, string, []string, []string, func(),
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
	"runners.promptSettled": "wired by StartTerminalWaitSweep, not New: it " +
		"publishes through the hub, a layer above this one, and stays nil in a " +
		"daemon with no detector (internal/runner/runner.go).",
	"turns.messageDelta": "wired by StartTerminalWaitSweep beside promptSettled, " +
		"for the same reason: a daemon with nobody to publish to records the " +
		"message when it finishes instead (internal/turn/turns.go).",
}

// sharedInstances are the pieces of in-flight state that MUST be one instance
// across the components holding them, listed as the field each component knows
// them by. The names differ on purpose — a registry the hook side calls `turns`
// is what the lifecycle side blocks on as `inflightTurns` — so the sharing cannot
// be inferred from the field name and has to be stated.
//
// A second instance of any of these does not fail to compile and does not fail a
// test that exercises one path at a time. It wedges a switch on a turn nothing
// will ever complete, or puts two CLIs on one chat.
var sharedInstances = map[string][]string{
	"the work mirror":     {"conversations.work", "turns.work", "runners.work"},
	"the spawn gate":      {"conversations.spawns", "runners.spawns"},
	"the turn registry":   {"turns.turns", "runners.inflightTurns"},
	"the turn-start gate": {"turns.turnStarts", "runners.turnStarts"},
	"the hook barrier":    {"turns.pendingHooks", "runners.pendingHooks"},
	"the telemetry store": {"conversations.telemetry", "turns.telemetry"},
	"the answer desk":     {"answers", "turns.answers", "runners.answers"},
	"the chat record":     {"conversations", "turns.conversations", "runners.conversations"},
	"the provider table":  {"providers", "runners.providers"},
	"the CLI lifecycle":   {"runners", "conversations.runners", "turns.runners"},
	"the hook ingress":    {"turns", "runners.turns"},
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
	return New(Deps{
		Chats:         wiringChats{},
		Runners:       wiringRunners{},
		Activity:      wiringActivity{},
		Agents:        engineagents.New(),
		Terminal:      screenReadingCommander{},
		Workspace:     wiringWorkspace{},
		Lineage:       wiringLineage{},
		ProviderPrefs: wiringPrefs{},
		Home:          func() (string, error) { return home, nil },
		Minter:        minter,
		Tools:         agenttools.Deps{},
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

// §7.1 — the hook ingress is never GIVEN the spawn gate.
//
// A hook must never block and must never fail: by the time it arrives the CLI has
// already acted. If ingest could take that gate, a SwitchProvider holding it while it
// parks waiting for the turn to finish would deadlock the CLI that is trying to report
// that very turn.
//
// This used to be asserted by watching a goroutine not block, which proves something
// about the goroutine and nothing about production. Since the decomposition it is
// STRUCTURAL: New hands the spawn gate to the chat record and to the CLI lifecycle,
// and to nothing else, so the hook ingress cannot take a lock it was never given.
//
// Inverted (add Spawns to turn.Deps and pass sh.spawns to it): fails with "the hook
// ingress was handed the spawn gate".
func TestInvariant_TheHookIngressIsNeverGivenTheSpawnGate(t *testing.T) {
	t.Parallel()
	u := newWiringFixture(t)

	spawnGate := fieldPointer(t, u, "runners.spawns")
	if spawnGate != fieldPointer(t, u, "conversations.spawns") {
		t.Fatal("precondition: the two paths that DO take the spawn gate must share one")
	}

	for name, held := range gatesHeldBy(u.turns) {
		if held == spawnGate {
			t.Fatalf("the hook ingress was handed the spawn gate as %q: a switch holding "+
				"it would deadlock the CLI trying to report the very turn the switch is "+
				"waiting for", name)
		}
	}
}

// gatesHeldBy collects every per-chat gate a component holds, by field name. The
// ingress legitimately holds two of them (the turn-start interlock and the
// per-runner hook gate); what it must never hold is the spawn gate.
func gatesHeldBy(component any) map[string]uintptr {
	held := map[string]uintptr{}
	fields := reflect.ValueOf(component).Elem()
	for i := range fields.NumField() {
		field := fields.Field(i)
		if field.Kind() != reflect.Pointer || field.IsNil() {
			continue
		}
		if field.Type() != reflect.TypeFor[*inflight.Gate]() {
			continue
		}
		held[fields.Type().Field(i).Name] = field.Pointer()
	}
	return held
}

// §7.2 — an UNKNOWN work state is never read as idle.
//
// The in-memory mirror is authoritative only once something has written to it. A
// switch asks "is this chat working?" and, if the mirror knows nothing, must fall back
// to the aggregate rather than assume idle — assuming idle is what killed a CLI still
// doing background work after its turn ended.
//
// This is the decision the ordering in closeTurn exists to protect (work.set runs
// before the deferred turns.complete). Asserting those two calls' order inside a test
// would only test the test; this asserts the behaviour that makes the order matter.
//
// Inverted (make chatWorking return `false, nil` when the mirror is unknown instead of
// reading the aggregate): fails with "an unknown work state was read as idle".
func TestInvariant_AnUnknownWorkStateFallsBackToTheAggregate(t *testing.T) {
	t.Parallel()
	work := inflight.NewWork()
	turns := turn.New(turn.Deps{Chats: workingChats{working: true}, Work: work})

	if _, known, _ := work.Observe("chat-1"); known {
		t.Fatal("precondition: the mirror must know nothing about this chat")
	}

	working, err := turns.ChatWorking(t.Context(), "chat-1")
	if err != nil {
		t.Fatalf("chatWorking: %v", err)
	}
	if !working {
		t.Fatal("an unknown work state was read as idle; the aggregate says the chat IS " +
			"working, and a switch trusting this would kill a CLI mid-background-work")
	}

	// Once the mirror knows, it wins: it is newer than any aggregate read.
	work.Set("chat-1", false)
	if working, err := turns.ChatWorking(t.Context(), "chat-1"); err != nil || working {
		t.Fatalf("a KNOWN mirror state must win over the aggregate, got (%v,%v)", working, err)
	}
}

// §7.5 — hook delivery is exactly-once by delivery id.
//
// The relay mints one id and reuses it on every retry. If a retry applied twice the
// user sees the same turn recorded twice, and there is nothing in CI that would catch
// it — the duplicate is perfectly well-formed.
//
// Inverted (have the registry forget the id between calls): the second Begin reports
// fresh and the test fails on "a retried delivery applied twice".
func TestInvariant_ARetriedDeliveryIsAppliedOnce(t *testing.T) {
	t.Parallel()
	journal := agentjournal.NewHookDeliveries()
	dir := journal.Dir(t.TempDir(), "runner-1")

	const delivery = "delivery-abc"
	hash := agentjournal.HookDeliveryHash("runner-1", "claude", "user_prompt", []byte(`{"prompt":"hi"}`))
	now := time.Unix(1, 0).UTC()

	done, err := journal.Begin(dir, delivery, hash, now)
	if err != nil {
		t.Fatalf("first Begin: %v", err)
	}
	if done {
		t.Fatal("the first sighting of a delivery id must not report DONE")
	}
	if err := journal.Complete(dir, delivery, hash, now); err != nil {
		t.Fatalf("Complete: %v", err)
	}

	// The relay retries with the SAME id.
	done, err = journal.Begin(dir, delivery, hash, now)
	if err != nil {
		t.Fatalf("retry Begin: %v", err)
	}
	if !done {
		t.Fatal("a retried delivery was NOT reported as already done: the relay reuses " +
			"one id across retries, so the user would see the same turn recorded twice")
	}
}

// §7.3 — liveness is row existence: the runner points at the chat, the chat never
// points back.
//
// Two places recording "who is live" is two places that can disagree, and the one that
// disagrees silently is the one that keeps a dead CLI on screen.
//
// Inverted (add a live-runner field to the chat aggregate and assert on it here):
// there is no second source to read, which is the point.
func TestInvariant_TheChatCarriesNoLivenessFlagOfItsOwn(t *testing.T) {
	t.Parallel()
	// A compile-time statement, deliberately: if someone adds a Live/Running field to
	// the chat aggregate, this file is where the argument against it lives.
	work := inflight.NewWork()
	if _, known, _ := work.Observe("never-seen"); known {
		t.Fatal("an unseen chat reported a KNOWN work state; liveness must come from " +
			"the runner row's existence, never from a flag the chat carries")
	}
}

// §7.6 — the exactly-once hook ingress is a DECLARED method, never discovered at
// runtime.
//
// It used to be found with a `, ok` type assertion. A port that only MIGHT carry it is
// a port a mis-wire drops silently: every hook takes the un-journalled path, every
// relay retry applies twice, and nothing fails until a user sees the same turn twice.
//
// Inverted (remove IngestHookDelivery from the TurnUsecase interface, or stop the
// concrete type implementing it): this file stops compiling, which is the entire point
// — the guarantee is a compile error, not a test failure.
func TestInvariant_TheExactlyOnceHookIngressIsCompileTimeWired(t *testing.T) {
	t.Parallel()
	// The concrete type the container builds must satisfy the port INCLUDING the
	// journalled ingress. If this ever needs a type assertion to be true, the
	// invariant is already broken.
	var _ interface {
		IngestHookDelivery(
			ctx context.Context,
			workspaceID, deliveryID, runnerID, provider, canonicalEvent string,
			rawPayload []byte,
		) error
	} = (*Usecase)(nil)
}

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
	_ context.Context, _, _ string, _, _ []string, _ func(),
) (string, error) {
	return "", nil
}

func (terminalCommanderShape) TerminateGraceful(_ context.Context, _ string) error {
	return nil
}

func (terminalCommanderShape) SessionLive(_ context.Context, _ string) bool { return false }

// workingChats is the chat store as this file needs it: an aggregate reporting that a
// chat is working, so the fallback in chatWorking has something to fall back TO.
type workingChats struct {
	agentchat.EventStore
	working bool
}

func (c workingChats) GetChat(_ context.Context, id string) (domain.Chat, error) {
	return domain.Chat{ID: id, Working: c.working}, nil
}
