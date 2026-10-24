package ingress

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	ledgerstore "github.com/char2cs/crowbar/api/internal/adapter/store/ledger"
	"github.com/char2cs/crowbar/api/internal/app/apperr"
	agentchat "github.com/char2cs/crowbar/api/internal/app/repositories/chat"
	"github.com/char2cs/crowbar/api/internal/app/usecases/chat/internal/shared/answerdesk"
	"github.com/char2cs/crowbar/api/internal/app/usecases/chat/internal/shared/promptsigil"
	"github.com/char2cs/crowbar/api/internal/app/usecases/chat/internal/shared/seam"
	"github.com/char2cs/crowbar/api/internal/app/usecases/chat/internal/shared/telemetry"
	"github.com/char2cs/crowbar/api/internal/core/paths/worktreepath"
	"github.com/char2cs/crowbar/api/internal/domain"
	"github.com/char2cs/crowbar/api/internal/domain/ledger"
	engineagents "github.com/char2cs/crowbar/api/internal/engine/agents"
)

// errChatGone is a hook from a runner placed on no chat, or on one deleted
// under it: nothing records it, and a CLI's turn is never resurrected by one.
var errChatGone = fmt.Errorf("agent: hook: the runner's chat is gone: %w", apperr.ErrNotFound)

// RecorderRunners is what a hook needs from the runner lifecycle. Nothing here
// may take the chat's lock: a hook is recorded under it already.
type RecorderRunners interface {
	// TurnRunner runs open on the runner a hook names, held against its end,
	// and reports false without running it once that end is under way.
	TurnRunner(runnerID string, open func(engineagents.Runner) error) (opened bool, err error)
	// HandleSessionStart applies a CLI's session announcement to its chat.
	HandleSessionStart(ctx context.Context, runner engineagents.Runner, ev engineagents.CanonicalEvent) error
	// TelemetryOnChatSurface reports whether the chat's surface carries its
	// provider's usage report at all.
	TelemetryOnChatSurface(ctx context.Context, chatID string) bool
	// ShowingNativeView reports whether runnerID is handed over to its
	// provider's own view right now.
	ShowingNativeView(runnerID string) bool
}

// Titles retitles a chat; a hook only ever supplies a derived title.
type Titles interface {
	RenameChat(ctx context.Context, chatID, title, source string) error
}

// RecorderDeps is everything the Recorder is built over.
type RecorderDeps struct {
	Chats     agentchat.EventStore
	Agents    engineagents.Agents
	Workspace seam.WorkspaceReader
	Home      func() (string, error)
	Ledger    *ledgerstore.Store
	Blobs     *ledgerstore.Blobs
	Telemetry *telemetry.Store
	Answers   *answerdesk.Desk
	Titles    Titles
}

// Recorder records what a CLI's hooks report as rows of its chat's ledger.
type Recorder struct {
	chats     agentchat.EventStore
	agents    engineagents.Agents
	ws        seam.WorkspaceReader
	home      func() (string, error)
	ledger    *ledgerstore.Store
	blobs     *ledgerstore.Blobs
	telemetry *telemetry.Store
	answers   *answerdesk.Desk
	titles    Titles
	// runners is bound after construction: the two sides call each other.
	runners RecorderRunners
	live    *live
	manual  *manualCompactions
	// feed is wired at sweep start; until then a frame has nobody to reach.
	feed atomic.Pointer[seam.ChatFeed]
}

// NewRecorder builds the Recorder. The runner port is bound by SetRunners.
func NewRecorder(d RecorderDeps) *Recorder {
	return &Recorder{
		chats: d.Chats, agents: d.Agents, ws: d.Workspace, home: d.Home, ledger: d.Ledger,
		blobs: d.Blobs, telemetry: d.Telemetry, answers: d.Answers, titles: d.Titles,
		live: newLive(), manual: &manualCompactions{chats: map[string]bool{}},
	}
}

// SetRunners binds the runner lifecycle.
func (r *Recorder) SetRunners(runners RecorderRunners) { r.runners = runners }

// SetFeed wires the live chat feed.
func (r *Recorder) SetFeed(feed seam.ChatFeed) { r.feed.Store(&feed) }

// ForgetChat drops what the Recorder holds of chatID: the chat is being erased.
func (r *Recorder) ForgetChat(chatID string) {
	r.live.forgetChat(chatID)
	r.manual.consume(chatID)
}

// Descriptor resolves the descriptor runner's hooks are parsed with.
func (r *Recorder) Descriptor(ctx context.Context, runner engineagents.Runner) (engineagents.Agent, error) {
	crowbarHome, _, _, _, err := r.ws.WorktreeDir(ctx, runner.WorkspaceID)
	if err != nil {
		return nil, fmt.Errorf("agent: hook: worktree dir: %w", err)
	}
	agent, err := r.agents.Get(ctx, crowbarHome, runner.ProviderID)
	if err != nil {
		return nil, fmt.Errorf("agent: hook: resolve descriptor: %w", err)
	}
	return agent, nil
}

// mapped are the events the mapper decides rows for.
var mapped = []string{
	engineagents.HookUserPrompt, engineagents.HookTurnStop, engineagents.HookTurnFailed, engineagents.HookIdle,
	engineagents.HookMessageDelta, engineagents.HookMessageComplete, engineagents.HookReasoningDelta,
	engineagents.HookToolOutputDelta, engineagents.HookDiffUpdate, engineagents.HookPlanUpdate,
	engineagents.HookToolPre, engineagents.HookToolPost, engineagents.HookToolFail,
	engineagents.HookSubagentPre, engineagents.HookSubagentPost, engineagents.HookNotification,
	engineagents.HookPermission, engineagents.HookElicitation,
	engineagents.HookCompactPre, engineagents.HookCompactPost,
}

// Dispatch records ev, one hook of runner's parsed by agent, on runner's chat.
// The caller holds the chat's lock. It returns once the rows are committed,
// with the stdout the hook's sender must print and the ask its answer is held on.
func (r *Recorder) Dispatch(
	ctx context.Context,
	runner engineagents.Runner,
	agent engineagents.Agent,
	ev engineagents.CanonicalEvent,
	raw []byte,
	channel engineagents.Channel,
) (reply []byte, ask string, err error) {
	if r.surfaceGated(runner.ID, agent, ev.Kind) {
		return nil, "", nil
	}
	chat, err := r.chatForRunner(ctx, runner)
	if err != nil {
		return nil, "", err
	}
	st, err := r.ledger.State(ctx, chat.ID)
	if err != nil {
		return nil, "", fmt.Errorf("agent: ingest hook: read the chat's bound session: %w", err)
	}
	src := Source{
		RunnerID: runner.ID, Provider: runner.ProviderID, SessionID: boundSession(st, runner.ProviderID),
		Persists: agent.SessionPersists(), Channel: channel,
	}
	if namesAnotherConversation(src.SessionID, ev) {
		// A nested conversation the chat's agent opened rides the same wire;
		// any other session is foreign.
		if !st.ScopeOpen(ev.SessionID) {
			return nil, "", fmt.Errorf("agent: hook: session %s: %w", ev.SessionID, engineagents.ErrForeignConversation)
		}
		src.Scope = ev.SessionID
	}
	switch {
	case src.Scope == "" && ev.Kind == engineagents.HookSessionStart:
		return nil, "", r.runners.HandleSessionStart(ctx, runner, ev)
	case ev.Kind == engineagents.HookSessionEnd:
		// A runner's end is recorded from its process exit, whether or not this fired.
		return nil, "", nil
	case !slices.Contains(mapped, ev.Kind):
		return nil, "", fmt.Errorf("agent: hook: %s: %w", ev.Kind, seam.ErrHookNotRecorded)
	}
	capability := r.prepare(ctx, chat, runner, agent, ev, raw, &src)
	fx, err := r.commit(ctx, chat, runner.ID, ev, src, r.putBlobs(ctx, ev))
	if err != nil {
		return nil, "", fmt.Errorf("agent: hook: %s: %w", ev.Kind, err)
	}
	if fx.HoldAsk != "" {
		r.answers.Hold(answerdesk.Prompt{
			ChoiceID: fx.HoldAsk, ChatID: chat.ID, RunnerID: runner.ID, Event: ev.Kind,
			Raw: append([]byte(nil), raw...), Keys: capability,
		})
	}
	return fx.Reply, fx.HoldAsk, nil
}

// prepare reads into src what the mapper cannot know itself: whose words a
// prompt is, whether an ask's reply can be held, how a compaction is asked
// for, and the text streamed so far. It returns the ask's answer capability.
func (r *Recorder) prepare(
	ctx context.Context, chat domain.Chat, runner engineagents.Runner, agent engineagents.Agent,
	ev engineagents.CanonicalEvent, raw []byte, src *Source,
) engineagents.AnswerCapability {
	src.Steer = func(p ledger.Prompt) ([]byte, bool) {
		stdout, ok := agent.PromptSteer(cmp.Or(p.Dispatch, p.Text))
		return []byte(stdout), ok && src.Channel == engineagents.ChannelHooks
	}
	var capability engineagents.AnswerCapability
	switch ev.Kind {
	case engineagents.HookUserPrompt:
		src.Prompt = r.promptOf(ctx, chat, runner, agent, ev.Message)
	case engineagents.HookPermission, engineagents.HookElicitation:
		var ok bool
		capability, ok = agent.AnswerCapability(ev.Kind)
		// Handed over to the provider's own view, the CLI asks there itself.
		src.Answerable = ok && len(raw) <= answerdesk.MaxPayloadBytes && !r.runners.ShowingNativeView(runner.ID)
	case engineagents.HookCompactPre, engineagents.HookCompactPost:
		wire, _, ok := agent.OutboundCall(compactStartEvent, nil)
		src.CompactsByPrompt = ok && wire == "prompt"
		src.Manual = ev.Kind == engineagents.HookCompactPre && r.manual.consume(chat.ID)
	case engineagents.HookTurnStop, engineagents.HookTurnFailed, engineagents.HookIdle:
		src.Live = r.unrecorded(chat.ID, runner.ID)
	}
	return capability
}

// compactStartEvent is the outbound event a provider asks to compact with.
const compactStartEvent = "compact_start"

func (r *Recorder) unrecorded(chatID, runnerID string) Unrecorded {
	return Unrecorded{
		Answers:   r.live.unrecorded(chatID, runnerID, "", KindAnswer),
		Reasoning: r.live.unrecorded(chatID, runnerID, "", KindReasoning),
		Diff:      r.live.unrecorded(chatID, runnerID, "", KindDiff),
	}
}

// promptOf reads whose words a user_prompt carries. Crowbar's own injected
// context is nobody's; the provider's harness speaking is its own; what is
// left is a person's, restored to the text Crowbar recorded for it.
func (r *Recorder) promptOf(
	ctx context.Context, chat domain.Chat, runner engineagents.Runner, agent engineagents.Agent, text string,
) Prompt {
	remainder, injected := r.agents.ConsumeInjectedPrefix(runner.ID, text)
	switch {
	case injected && remainder == "":
		return Prompt{}
	case injected:
		text = remainder
	case harnessSaid(agent, text):
		return Prompt{Text: text, Origin: ledger.OriginHarness}
	}
	if chatsDir, err := r.ws.AgentChatsDir(ctx, runner.WorkspaceID); err != nil {
		slog.WarnContext(ctx, "agent: ingest hook: resolve chats dir for attachment ref restore",
			"chat_id", chat.ID, "runner_id", runner.ID, "err", err)
	} else {
		text = worktreepath.RestoreDurableAttachmentRefs(text, chatsDir, chat.ID)
	}
	sigils, escape := agent.PromptLeadingSigils()
	text = promptsigil.Strip(sigils, escape, chat.ID, text)
	if err := r.titles.RenameChat(ctx, chat.ID, deriveTitle(text), "derived"); err != nil {
		slog.WarnContext(ctx, "agent: ingest hook: derived title", "err", err, "chat_id", chat.ID)
	}
	return Prompt{Text: text, Origin: ledger.OriginProvider}
}

func harnessSaid(agent engineagents.Agent, text string) bool {
	_, ok := engineagents.MatchInjectedPrompt(agent, text)
	return ok
}

// commit decides and commits ev's rows. A turn opens only on a runner held
// against its end; one whose end is under way records its event unheld.
func (r *Recorder) commit(
	ctx context.Context, chat domain.Chat, runnerID string, ev engineagents.CanonicalEvent, src Source, refs BlobRefs,
) (Effects, error) {
	var fx Effects
	do := func() error {
		_, _, err := r.ledger.Do(ctx, chat.ID, func(st ledger.State) ([]ledger.Row, error) {
			rows, effects, err := Map(st, ev, src, refs)
			fx = effects
			if err == nil && effects.Live != nil {
				// Shown before its rows commit, so the settled text a commit
				// publishes is the last word on the message.
				r.publishLive(chat, runnerID, *effects.Live)
			}
			return rows, err
		})
		return err
	}
	var err error
	if ev.Kind == engineagents.HookUserPrompt || ev.Kind == engineagents.HookTurnStop {
		err = r.held(runnerID, &src, do)
	} else {
		err = do()
	}
	if err != nil {
		return Effects{}, err
	}
	if fx.Ended != "" {
		r.live.forget(chat.ID, runnerID, fx.Ended)
	}
	return fx, nil
}

// held runs do with runnerID held against its end, or unheld when its end is
// under way.
func (r *Recorder) held(runnerID string, src *Source, do func() error) error {
	opened, err := r.runners.TurnRunner(runnerID, func(engineagents.Runner) error {
		src.Held = true
		return do()
	})
	if opened {
		return err
	}
	src.Held = false
	return do()
}

func (r *Recorder) publishLive(chat domain.Chat, runnerID string, d LiveDelta) {
	text, startedAt := r.live.observe(chat.ID, runnerID, d, time.Now())
	feed := r.feed.Load()
	if feed == nil || feed.MessageDelta == nil {
		return
	}
	if d.Kind != KindAnswer {
		startedAt = time.Time{}
	}
	feed.MessageDelta(chat.ID, chat.WorkspaceID, d.ShownAs, text, d.Kind, startedAt)
}

// ExitRows are what runnerID's exit records of the answers it streamed with no
// row of their own; the caller commits them with the exit, then calls Retire.
func (r *Recorder) ExitRows(st ledger.State, chatID, runnerID string) ([]ledger.Row, error) {
	return Salvage(st, r.live.unrecorded(chatID, runnerID, "", KindAnswer))
}

// Retire drops runnerID's streamed text on chatID: its exit is recorded.
func (r *Recorder) Retire(chatID, runnerID string) { r.live.forget(chatID, runnerID, "") }

// surfaceGated reports whether canonical opts out of the surface in front of
// the user. An event that declares no surfaces is on every one.
func (r *Recorder) surfaceGated(runnerID string, descriptor engineagents.Agent, canonical string) bool {
	surfaces := descriptor.EventSurfaces(canonical)
	if len(surfaces) == 0 {
		return false
	}
	surface := engineagents.SurfaceChat
	if r.runners.ShowingNativeView(runnerID) {
		surface = engineagents.SurfaceTerminal
	}
	return !slices.Contains(surfaces, surface)
}

// chatForRunner is the chat runner is on, errChatGone when it is on none or
// that chat was deleted under it (the CLI is still dying).
func (r *Recorder) chatForRunner(ctx context.Context, runner engineagents.Runner) (domain.Chat, error) {
	if runner.CurrentChatID == "" {
		return domain.Chat{}, errChatGone
	}
	chat, err := r.chats.GetChat(ctx, runner.CurrentChatID)
	if errors.Is(err, agentchat.ErrNotFound) {
		return domain.Chat{}, errChatGone
	}
	if err != nil {
		return domain.Chat{}, fmt.Errorf("agent: hook: chat: %w", err)
	}
	return chat, nil
}

// RecordTelemetry records a usage report runner sent over its hook channel. It
// is no conversation event, so it is never judged by session.
func (r *Recorder) RecordTelemetry(
	ctx context.Context, runner engineagents.Runner, agent engineagents.Agent, raw []byte,
) error {
	chat, err := r.chatForRunner(ctx, runner)
	if err != nil {
		return err
	}
	report, err := agent.ParseTelemetry(raw, time.Now())
	if err != nil {
		return fmt.Errorf("agent: telemetry: %w: %w", apperr.ErrUnprocessable, err)
	}
	if report.Empty() {
		return nil
	}
	r.telemetry.Set(ctx, chat.ID, report)
	feed := r.feed.Load()
	if feed != nil && feed.Telemetry != nil && r.runners.TelemetryOnChatSurface(ctx, chat.ID) {
		feed.Telemetry(chat.ID, chat.WorkspaceID, report)
	}
	return nil
}

// Telemetry is the chat provider's last usage report.
func (r *Recorder) Telemetry(chatID string) (engineagents.Telemetry, bool) {
	return r.telemetry.Get(chatID)
}

// ArmManualCompaction records that chatID's next compaction was asked for by
// a person, for a provider whose own report never says so.
func (r *Recorder) ArmManualCompaction(chatID string) { r.manual.arm(chatID) }

// manualCompactions are the chats whose next compaction a person asked for.
// A request that never reaches its provider leaves its chat armed, and the
// next compaction reads as asked for: the provider says no better.
type manualCompactions struct {
	mu    sync.Mutex
	chats map[string]bool
}

func (m *manualCompactions) arm(chatID string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.chats[chatID] = true
}

func (m *manualCompactions) consume(chatID string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	armed := m.chats[chatID]
	delete(m.chats, chatID)
	return armed
}

func deriveTitle(prompt string) string {
	for _, line := range strings.Split(prompt, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if r := []rune(line); len(r) > 60 {
			return strings.TrimSpace(string(r[:60])) + "…"
		}
		return line
	}
	return ""
}
