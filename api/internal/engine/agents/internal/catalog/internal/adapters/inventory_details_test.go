package adapters

import (
	"context"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/engine/agents/internal/catalog/internal/normalize"
	"github.com/char2cs/crowbar/api/internal/engine/agents/internal/spec"
)

// stubRunner replies to a fixed set of argv strings, and records every call it
// received — used here to prove which detail commands a probe DID and did NOT
// issue.
type stubRunner struct {
	mu      sync.Mutex
	replies map[string][]byte
	calls   []string
}

func (r *stubRunner) Run(_ context.Context, argv []string) ([]byte, error) {
	key := strings.Join(argv, " ")
	r.mu.Lock()
	defer r.mu.Unlock()
	r.calls = append(r.calls, key)
	return r.replies[key], nil
}

func (r *stubRunner) callsSeen() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]string(nil), r.calls...)
}

func TestSplitItems_SkipsBlankNamesAfterTrimming(t *testing.T) {
	got := splitItems("one, , two ,  ,three", ",", row{id: "x", source: "src"}, 10)

	names := make([]string, 0, len(got))
	for _, c := range got {
		names = append(names, c.Name)
	}
	assert.Equal(t, []string{"one", "two", "three"}, names,
		"a blank entry left by a stray separator must be dropped, never surfaced as an empty candidate")
}

func TestSourceOf_EmptyPatternFallsBackToNormalizeSource(t *testing.T) {
	got := sourceOf("plugin@registry", "")
	assert.Equal(t, normalize.Source("plugin@registry"), got)
}

func TestSourceOf_InvalidRegexPatternYieldsEmptySource(t *testing.T) {
	got := sourceOf("plugin@registry", "(unterminated")
	assert.Empty(t, got, "a broken source_pattern in the descriptor must degrade to no source, never panic")
}

func TestSourceOf_PatternWithNoMatchYieldsEmptySource(t *testing.T) {
	got := sourceOf("plugin@registry", `^nomatch$`)
	assert.Empty(t, got)
}

func TestWarn_EmptySourceUsesGenericPhrasing(t *testing.T) {
	got := warn("", "could not be inspected")
	assert.Equal(t, "One catalog source could not be inspected.", got)
}

func TestEmptyInventory_EmptyPatternNeverMatches(t *testing.T) {
	assert.False(t, emptyInventory("", []byte("Skills (0): none")),
		"a descriptor with no detail_empty_pattern must never claim a match")
}

func TestEmptyInventory_InvalidPatternNeverMatches(t *testing.T) {
	assert.False(t, emptyInventory("(unterminated", []byte("Skills (0): none")),
		"a broken detail_empty_pattern must degrade to no match, never panic")
}

// TestProbe_RowsBeyondTheItemCeilingAreTruncatedBeforeFanOut pins that the row
// truncation happens BEFORE any detail command is issued: a provider inventory
// with more rows than the item ceiling must warn and drop the excess rows
// up front, not fan out to every row and truncate the resulting items.
func TestProbe_RowsBeyondTheItemCeilingAreTruncatedBeforeFanOut(t *testing.T) {
	s := &spec.SlashCatalogSpec{
		MaxItems: 1,
		Pipeline: spec.CatalogPipelineSpec{
			Command:          []string{"list"},
			RowsPath:         "[]",
			EnabledField:     "enabled",
			IDField:          "id",
			DetailCommand:    []string{"detail", "{id}"},
			DetailPattern:    `(?s)^Skills:(?P<items>.*)$`,
			DetailItemsGroup: "items",
			DetailSeparator:  ",",
		},
	}
	runner := &stubRunner{replies: map[string][]byte{
		"list":       []byte(`[{"id":"a@x","enabled":true},{"id":"b@x","enabled":true},{"id":"c@x","enabled":true}]`),
		"detail a@x": []byte("Skills:one"),
	}}

	got, err := inventoryDetails{}.Probe(context.Background(), s, runner)

	require.NoError(t, err)
	assert.Contains(t, strings.Join(got.Warnings, "|"), "safe expansion limit",
		"exceeding the row ceiling must warn that sources were omitted")
	calls := runner.callsSeen()
	assert.NotContains(t, calls, "detail b@x", "a row truncated away before fan-out must never be inspected")
	assert.NotContains(t, calls, "detail c@x", "a row truncated away before fan-out must never be inspected")
}

// gateRunner answers the listing call immediately, but blocks every detail
// call on the caller's context — closing entered (once) the moment the first
// one is called, so a test can cancel the context only once it knows a detail
// goroutine is actually holding the fan-out's one concurrency slot.
//
// once guards the close: once the slot-holder's Run unblocks on ctx.Done() and
// releases its slot, the goroutine that had been parked waiting for that same
// slot may ALSO win the race between the now-ready slot and the now-closed
// ctx.Done() (select picks uniformly among ready cases) and call Run itself —
// a second, harmless call this fixture must not panic on.
type gateRunner struct {
	listing []byte
	entered chan struct{}
	once    sync.Once
}

func (g *gateRunner) Run(ctx context.Context, argv []string) ([]byte, error) {
	if argv[0] == "list" {
		return g.listing, nil
	}
	g.once.Do(func() { close(g.entered) })
	<-ctx.Done()
	return nil, ctx.Err()
}

// TestProbe_ContextCancelledDuringFanOutIsPropagated pins BOTH halves of the
// fan-out cancellation contract: a goroutine still waiting for a concurrency
// slot when the context dies must give up on ctx.Done() rather than proceed
// (fanOut's own select), and Probe must then report the cancellation rather
// than assembling a result from whatever fanOut managed to collect.
//
// DetailConcurrency: 1 with two rows guarantees one goroutine holds the only
// slot (blocked in gateRunner.Run on ctx.Done()) while the other is parked in
// fanOut's select, unable to acquire it until the holder releases — which
// happens only once the context is cancelled. No timing dependency: the
// outcome is forced by the concurrency limit, not by scheduling luck.
func TestProbe_ContextCancelledDuringFanOutIsPropagated(t *testing.T) {
	s := &spec.SlashCatalogSpec{
		Pipeline: spec.CatalogPipelineSpec{
			Command:           []string{"list"},
			RowsPath:          "[]",
			EnabledField:      "enabled",
			IDField:           "id",
			DetailCommand:     []string{"detail", "{id}"},
			DetailPattern:     `(?s)^Skills:(?P<items>.*)$`,
			DetailItemsGroup:  "items",
			DetailSeparator:   ",",
			DetailConcurrency: 1,
		},
	}
	runner := &gateRunner{
		listing: []byte(`[{"id":"a@x","enabled":true},{"id":"b@x","enabled":true}]`),
		entered: make(chan struct{}),
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	type outcome struct {
		result Result
		err    error
	}
	done := make(chan outcome, 1)
	go func() {
		result, err := inventoryDetails{}.Probe(ctx, s, runner)
		done <- outcome{result, err}
	}()

	<-runner.entered
	cancel()
	out := <-done

	assert.ErrorIs(t, out.err, context.Canceled,
		"a context cancelled mid fan-out must surface as cancellation, not a partial result")
}
