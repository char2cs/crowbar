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

	ledgerstore "github.com/char2cs/crowbar/api/internal/adapter/store/ledger"
	agentchat "github.com/char2cs/crowbar/api/internal/app/repositories/chat"
	"github.com/char2cs/crowbar/api/internal/app/usecases/chat/internal/shared/answerdesk"
	"github.com/char2cs/crowbar/api/internal/app/usecases/chat/internal/shared/promptsigil"
	"github.com/char2cs/crowbar/api/internal/app/usecases/chat/internal/shared/seam"
	"github.com/char2cs/crowbar/api/internal/app/usecases/chat/internal/shared/telemetry"
	"github.com/char2cs/crowbar/api/internal/core/paths/worktreepath"
	"github.com/char2cs/crowbar/api/internal/domain"
	"github.com/char2cs/crowbar/api/internal/domain/ledger"
	"github.com/char2cs/crowbar/api/internal/domain/problem"
	engineagents "github.com/char2cs/crowbar/api/internal/engine/agents"
)

var errChatGone = fmt.Errorf("agent: hook: the runner's chat is gone: %w", problem.ErrNotFound)

type RecorderRunners interface {
	TurnRunner(runnerID string, open func(engineagents.Runner) error) (opened bool, err error)
	HandleSessionStart(ctx context.Context, runner engineagents.Runner, ev engineagents.CanonicalEvent) error
	TelemetryOnChatSurface(ctx context.Context, chatID string) bool
	ShowingNativeView(runnerID string) bool
}

type Titles interface {
	RenameChat(ctx context.Context, chatID, title, source string) error
}

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
	Frames    Frames
}

type Frames interface {
	FeedDelta(chatID string, d ledger.FeedDelta)
	Telemetry(chatID, workspaceID string, report engineagents.Telemetry)
	TelemetryGone(chatID string)
}

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
	frames    Frames
	runners   RecorderRunners
	live      *live
	manual    *manualCompactions
	unbound   *held
}

func NewRecorder(d RecorderDeps) *Recorder {
	return &Recorder{
		chats: d.Chats, agents: d.Agents, ws: d.Workspace, home: d.Home, ledger: d.Ledger,
		blobs: d.Blobs, telemetry: d.Telemetry, answers: d.Answers, titles: d.Titles, frames: d.Frames,
		live: newLive(), manual: &manualCompactions{chats: map[string]bool{}}, unbound: newHeld(),
	}
}

func (r *Recorder) SetRunners(runners RecorderRunners) { r.runners = runners }

func (r *Recorder) ForgetChat(chatID string) {
	r.live.forgetChat(chatID)
	r.manual.consume(chatID)
	r.unbound.release(chatID)
}

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
	chat, st, err := r.chatState(ctx, runner)
	if err != nil {
		return nil, "", err
	}
	ev = r.withRunnerDir(ctx, runner, ev)
	if denial, ok := guardLeftFolder(st, agent, ev, raw, channel); ok {
		return denial, "", nil
	}
	src := Source{
		RunnerID: runner.ID, Provider: runner.ProviderID, SessionID: boundSession(st, runner.ProviderID),
		Persists: agent.SessionPersists(), Channel: channel,
	}
	frame := heldFrame{runner: runner, agent: agent, ev: ev, raw: raw, channel: channel}
	if held, err := r.placeScope(ctx, chat.ID, st, frame, &src); held || err != nil {
		return nil, "", err
	}
	switch {
	case src.Scope == "" && ev.Kind == engineagents.HookSessionStart && !startsOn(st, runner.ID):
		return nil, "", fmt.Errorf("agent: hook: a past runner's session: %w", seam.ErrHookNotRecorded)
	case src.Scope == "" && ev.Kind == engineagents.HookSessionStart:
		return nil, "", r.runners.HandleSessionStart(ctx, runner, ev)
	case acknowledgedOnly(ev, src):
		return nil, "", nil
	case !records(ev.Kind):
		return nil, "", fmt.Errorf("agent: hook: %s: %w", ev.Kind, seam.ErrHookNotRecorded)
	}
	capability := r.prepare(ctx, chat, runner, agent, ev, raw, &src)
	release := r.blobs.Naming()
	fx, err := r.commit(ctx, chat, runner.ID, ev, src, r.putBlobs(ctx, ev))
	release()
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

func (r *Recorder) withRunnerDir(
	ctx context.Context, runner engineagents.Runner, ev engineagents.CanonicalEvent,
) engineagents.CanonicalEvent {
	if ev.Tool == nil || ev.Tool.Cwd != "" {
		return ev
	}
	_, _, _, worktree, err := r.ws.WorktreeDir(ctx, runner.WorkspaceID)
	if err != nil || worktree == "" {
		return ev
	}
	tool := *ev.Tool
	tool.Cwd = worktree
	ev.Tool = &tool
	return ev
}

func (r *Recorder) chatState(ctx context.Context, runner engineagents.Runner) (domain.Chat, ledger.State, error) {
	chat, err := r.chatForRunner(ctx, runner)
	if err != nil {
		return domain.Chat{}, ledger.State{}, err
	}
	st, err := r.ledger.State(ctx, chat.ID)
	if err != nil {
		return domain.Chat{}, ledger.State{}, fmt.Errorf("agent: ingest hook: read the chat's bound session: %w", err)
	}
	return chat, st, nil
}

func acknowledgedOnly(ev engineagents.CanonicalEvent, src Source) bool {
	return ev.Kind == engineagents.HookSessionEnd || src.Scope != "" && ev.Kind == engineagents.HookSessionStart
}

func (r *Recorder) prepare(
	ctx context.Context, chat domain.Chat, runner engineagents.Runner, agent engineagents.Agent,
	ev engineagents.CanonicalEvent, raw []byte, src *Source,
) engineagents.AnswerCapability {
	src.Steer = func(p ledger.Prompt) ([]byte, bool) {
		stdout, ok := agent.PromptSteer(cmp.Or(p.Dispatch, p.Text))
		return []byte(stdout), ok && src.Channel == engineagents.ChannelHooks
	}
	src.Command = agent.PromptIsCommand
	src.EndsAtAction = agent.AnswerEndsAtAction()
	if endsText(ev.Kind) && src.EndsAtAction {
		src.Live.Answers = r.live.unrecorded(chat.ID, runner.ID, "", KindAnswer)
	}
	var capability engineagents.AnswerCapability
	switch ev.Kind {
	case engineagents.HookUserPrompt:
		src.Prompt = r.promptOf(ctx, chat, runner, agent, ev.Message)
	case engineagents.HookPermission, engineagents.HookElicitation:
		var ok bool
		capability, ok = agent.AnswerCapability(ev.Kind)
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

const compactStartEvent = "compact_start"

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
	if home, err := r.home(); err != nil {
		slog.WarnContext(ctx, "agent: ingest hook: resolve home for attachment ref restore",
			"chat_id", chat.ID, "runner_id", runner.ID, "err", err)
	} else {
		text = worktreepath.RestoreDurableAttachmentRefs(text, home, chat.ID)
	}
	sigils, escape := agent.PromptLeadingSigils()
	return Prompt{Text: promptsigil.Strip(sigils, escape, chat.ID, text), Origin: ledger.OriginProvider}
}

func harnessSaid(agent engineagents.Agent, text string) bool {
	_, ok := engineagents.MatchInjectedPrompt(agent, text)
	return ok
}

func (r *Recorder) commit(
	ctx context.Context, chat domain.Chat, runnerID string, ev engineagents.CanonicalEvent, src Source, refs BlobRefs,
) (Effects, error) {
	var fx Effects
	var committed []ledger.Row
	var after ledger.State
	do := func() error {
		var err error
		committed, after, err = r.ledger.Do(ctx, chat.ID, func(st ledger.State) ([]ledger.Row, error) {
			rows, effects, err := Map(st, ev, src, refs)
			fx = effects
			fx.Bound = boundScope(rows)
			return rows, err
		})
		return err
	}
	var err error
	if ev.Kind == engineagents.HookUserPrompt || ev.Kind == engineagents.HookTurnStop || ev.Kind == engineagents.HookSubagentResult {
		err = r.held(runnerID, &src, do)
	} else {
		err = do()
	}
	if err != nil {
		return Effects{}, err
	}
	if fx.Live != nil {
		r.publishLive(chat, runnerID, *fx.Live)
	}
	if len(committed) > 0 {
		r.live.dropRecorded(chat.ID, runnerID, after)
	}
	if fx.Title != "" {
		if err := r.titles.RenameChat(ctx, chat.ID, deriveTitle(fx.Title), "derived"); err != nil {
			slog.WarnContext(ctx, "agent: ingest hook: derived title", "err", err, "chat_id", chat.ID)
		}
	}
	if fx.Ended != "" {
		r.live.forget(chat.ID, runnerID, fx.Ended)
	}
	if fx.Bound != "" {
		r.drain(ctx, chat.ID, fx.Bound)
	}
	if !startingSubagent(after) {
		r.settleUnclaimed(ctx, chat.ID, r.unbound.release(chat.ID))
	}
	if fx.Unbound != "" {
		slog.WarnContext(ctx, "agent: ingest hook: an ask's input names none of its tool's open calls",
			"chat_id", chat.ID, "tool_name", ev.Choice.ToolName, "ask_id", fx.Unbound)
	}
	return fx, nil
}

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

func (r *Recorder) Mark(ctx context.Context, runner engineagents.Runner, row ledger.Row) error {
	chat, err := r.chatForRunner(ctx, runner)
	if err != nil {
		return err
	}
	if _, _, err := r.ledger.Append(ctx, chat.ID, row); err != nil {
		return fmt.Errorf("agent: hook: mark: %w", err)
	}
	return nil
}

func (r *Recorder) ArmManualCompaction(chatID string) { r.manual.arm(chatID) }

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
