package agent

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/char2cs/crowbar/api/internal/app/apperr"
	"github.com/char2cs/crowbar/api/internal/domain"
	engineagents "github.com/char2cs/crowbar/api/internal/engine/agents"
)

// The answer channel: how a human answers a provider's prompt from Crowbar's
// chat, with no TUI interaction.
//
// The whole mechanism rests on one measured fact. A vendor CLI blocks its own
// permission gate for as long as the hook process it fired is still running, and
// reads that process's stdout as the decision — measured against claude 2.1.234,
// where a hook held the dialog for 45.5 seconds and then answered it, the tool
// ran, and no human touched the terminal. So the relay does not poll for work: it
// stays alive, and the daemon hands it a verdict.
//
// Three properties are load-bearing, and each of them is a test:
//
//   - A prompt is only held open if the PROVIDER declares how to answer it. A
//     descriptor with no `answer:` block never opens a slot, the relay never
//     blocks, and every hook behaves exactly as it did before this file existed.
//
//   - A held hook is ALWAYS released. The wait has a deadline, a dead CLI
//     releases every slot it owned, and a relay killed by the provider reports
//     back before it dies. A prompt that hung forever would be worse than no
//     answer channel at all, because the chat would show a question nobody can
//     clear.
//
//   - NOTHING is rendered without a live waiter. Answering a prompt whose relay
//     has already gone is a lie: the CLI is showing its own dialog by then, and
//     Crowbar would report success for a decision that reached nobody.
const (
	// maxAnswerPayloadBytes bounds the hook payload a pending slot retains.
	//
	// It is retained at all because some answers are an EDIT of what the provider
	// sent rather than a verdict on it — claude answers AskUserQuestion by being
	// handed its own tool input back with the picks merged in — and the payload is
	// the only place that input exists. A prompt whose payload is larger than this
	// is observed exactly as before and simply is not answerable, which is the same
	// outcome as a provider that declares no answer channel.
	maxAnswerPayloadBytes = 128 << 10

	// answerWaitFallback is the wait used when a descriptor declares an answer
	// channel but no budget. It is finite on purpose: a slot with no deadline is a
	// relay that never exits.
	answerWaitFallback = 120 * time.Second

	// maxAnswerWait caps whatever a descriptor asks for. An overridden descriptor
	// on disk is user-editable, and a wait longer than any provider's own hook
	// timeout would just get the relay killed mid-write instead of letting it exit.
	maxAnswerWait = 15 * time.Minute
)

// PendingAnswer is what a relay is told to do after its hook has been recorded.
//
// It is returned from the ingest call rather than discovered by the relay,
// because the relay has no way to name the prompt: identity is minted here, from
// the payload, after the chat is resolved. The relay knows only its own delivery
// id, which is the key it polls back on.
type PendingAnswer struct {
	// ChoiceID is Crowbar's identity for the prompt now waiting on a human. The
	// relay never interprets it; it is carried so a log line can name what a hook
	// is blocked on.
	ChoiceID string
	// Wait is how long the daemon will hold the relay. The relay uses it to bound
	// its own request rather than inventing a number, so the budget lives in one
	// place — the descriptor.
	Wait time.Duration
}

// HookAnswer is the verdict handed back to a blocked relay.
//
// Stdout is what the relay must print, VERBATIM and in one write. Rendering it
// here rather than in the relay is deliberate: the relay forwards bytes and
// decides nothing, and the descriptor that knows a provider's JSON lives in the
// daemon. Empty Stdout means "print nothing" — the honest answer for a prompt
// that was resolved somewhere else.
type HookAnswer struct {
	Stdout []byte
}

// answerSlot is one relay held open on one prompt.
type answerSlot struct {
	choiceID string
	chatID   string
	runnerID string
	// event and raw are what the prompt arrived on, kept so the descriptor can
	// render an answer that echoes the provider's own payload back.
	event string
	raw   []byte
	// keys are the decision keys this provider can express for this event. An
	// answer outside them is refused before anything is written.
	keys engineagents.AnswerCapability

	done   chan struct{}
	stdout []byte
	once   sync.Once
}

// settle publishes a verdict and wakes the relay. It is idempotent: a prompt
// answered in Crowbar at the same moment its CLI dies must wake exactly one
// waiter with exactly one verdict.
func (s *answerSlot) settle(stdout []byte) {
	s.once.Do(func() {
		s.stdout = stdout
		close(s.done)
	})
}

// answerDesk holds every relay currently blocked on a human.
//
// It is in memory, and that is correct rather than a shortcut: a slot describes a
// LIVE process that is blocking a LIVE provider gate. One that outlived the
// daemon would be a promise to a relay that is already gone, and the relay's own
// deadline has released it long before any restart completes.
//
// There is no goroutine per prompt. A waiter is the HTTP request's own goroutine
// parked on a channel, so the population is bounded by the number of live CLIs
// and cancelling the request is what releases it.
type answerDesk struct {
	mu         sync.Mutex
	byChoice   map[string]*answerSlot
	byDelivery map[string]*answerSlot
}

func newAnswerDesk() *answerDesk {
	return &answerDesk{
		byChoice:   map[string]*answerSlot{},
		byDelivery: map[string]*answerSlot{},
	}
}

// open registers a relay as blocked. A delivery that already has a slot keeps it:
// the relay retries one delivery id until the daemon acknowledges it, and a retry
// must find the prompt it opened rather than a second copy of it.
func (d *answerDesk) open(deliveryID string, slot *answerSlot) *answerSlot {
	d.mu.Lock()
	defer d.mu.Unlock()
	if existing, held := d.byDelivery[deliveryID]; held {
		return existing
	}
	// A prompt re-opened under an identity that is already blocked releases the
	// older relay first. Two relays waiting on one prompt could each be handed the
	// same verdict, and the provider would be told twice.
	if stale, held := d.byChoice[slot.choiceID]; held {
		stale.settle(nil)
	}
	d.byDelivery[deliveryID] = slot
	d.byChoice[slot.choiceID] = slot
	return slot
}

func (d *answerDesk) byDeliveryID(deliveryID string) (*answerSlot, bool) {
	d.mu.Lock()
	defer d.mu.Unlock()
	slot, ok := d.byDelivery[deliveryID]
	return slot, ok
}

func (d *answerDesk) byChoiceID(choiceID string) (*answerSlot, bool) {
	d.mu.Lock()
	defer d.mu.Unlock()
	slot, ok := d.byChoice[choiceID]
	return slot, ok
}

// close takes a slot off the desk and wakes its relay with the given verdict.
func (d *answerDesk) close(slot *answerSlot, stdout []byte) {
	d.mu.Lock()
	for id, held := range d.byDelivery {
		if held == slot {
			delete(d.byDelivery, id)
		}
	}
	if held, ok := d.byChoice[slot.choiceID]; ok && held == slot {
		delete(d.byChoice, slot.choiceID)
	}
	d.mu.Unlock()
	slot.settle(stdout)
}

// releaseRunner frees every relay a dead CLI owned, and reports which prompts
// they were holding so the caller can clear them from the chat too.
//
// It exists because hooks are spawned DETACHED (measured against claude 2.1.234):
// killing a CLI leaves its hook running as an orphan, still blocked, still able
// to write to a terminal nobody is reading. The orphan's own deadline would
// eventually release it, but the CHAT must not show a pending question for a
// process that no longer exists — so the runner's death releases the slots at
// once.
func (d *answerDesk) releaseRunner(runnerID string) []*answerSlot {
	d.mu.Lock()
	var doomed []*answerSlot
	for _, slot := range d.byChoice {
		if slot.runnerID == runnerID {
			doomed = append(doomed, slot)
		}
	}
	for _, slot := range doomed {
		for id, held := range d.byDelivery {
			if held == slot {
				delete(d.byDelivery, id)
			}
		}
		delete(d.byChoice, slot.choiceID)
	}
	d.mu.Unlock()
	for _, slot := range doomed {
		slot.settle(nil)
	}
	return doomed
}

// holdForAnswer registers a relay against the prompt an event just opened, when
// the provider says the prompt can be answered at all.
//
// It is called from the observation path with the prompt already recorded, so the
// question is legible in the chat whether or not anybody is waiting on it. A
// provider with no answer channel, a payload too large to echo back, or a hook
// that reached the daemon by some route other than the relay all land on the same
// no-op — the prompt is observed, the relay is not held.
//
// HOLDING A RELAY COSTS THE HUMAN AT THE TERMINAL NOTHING, which is what makes it
// safe to hold one for a prompt the chat may never offer a button for (a
// permission that arrived with no turn open is recorded already-resolved, and
// nobody can answer it here). Measured against claude 2.1.234 on 2026-08-18: the
// CLI draws its own dialog WHILE the hook blocks — a Notification fired six
// seconds into a twelve-second hold — and a keystroke there wins immediately,
// with the hook's later answer discarded in silence. So the worst case of a hold
// nobody answers is the budget expiring, which is the behaviour of a machine with
// no Crowbar on it.
func (u *Usecase) holdForAnswer(
	ctx context.Context,
	chat domain.AgentChat,
	runner domain.AgentRunner,
	agent engineagents.Agent,
	ev engineagents.CanonicalEvent,
	choiceID string,
	raw []byte,
) {
	deliveryID := hookDeliveryID(ctx)
	if deliveryID == "" || choiceID == "" {
		return
	}
	capability, answerable := agent.AnswerCapability(ev.Kind)
	if !answerable {
		return
	}
	if len(raw) > maxAnswerPayloadBytes {
		slog.DebugContext(ctx, "agent: answer: prompt payload too large to answer",
			"chat_id", chat.ID, "event", ev.Kind, "bytes", len(raw))
		return
	}
	slot := u.answers.open(deliveryID, &answerSlot{
		choiceID: choiceID,
		chatID:   chat.ID,
		runnerID: runner.ID,
		event:    ev.Kind,
		// The payload is copied because the caller's buffer is the request body's,
		// and the slot outlives the request that opened it.
		raw:  append([]byte(nil), raw...),
		keys: capability,
		done: make(chan struct{}),
	})
	_ = slot
}

// PendingAnswer reports whether the relay that just delivered deliveryID must
// wait for a human, and for how long.
//
// It is a read taken AFTER ingestion rather than a value threaded through it: the
// ingest path is a long chain that must never fail a hook, and giving it a return
// value it could drop is a worse contract than a lookup that simply answers false.
func (u *Usecase) PendingAnswer(deliveryID string) (PendingAnswer, bool) {
	slot, held := u.answers.byDeliveryID(deliveryID)
	if !held {
		return PendingAnswer{}, false
	}
	return PendingAnswer{ChoiceID: slot.choiceID, Wait: answerWait(slot.keys.Wait)}, true
}

// answerWait clamps a descriptor's declared budget. Zero means the descriptor
// declared an answer channel and no budget, which is a mis-authored descriptor
// rather than a request for an unbounded wait.
func answerWait(declared time.Duration) time.Duration {
	switch {
	case declared <= 0:
		return answerWaitFallback
	case declared > maxAnswerWait:
		return maxAnswerWait
	default:
		return declared
	}
}

// AwaitAnswer blocks the relay until its prompt is decided, the wait expires, or
// the relay goes away.
//
// Every one of those three returns the SAME shape, and only the first carries
// bytes. That is the whole contract with the relay: it prints what it is given
// and nothing otherwise, so a timeout and a prompt resolved at the PTY both leave
// the provider's own dialog standing — which is exactly the behaviour of a
// Crowbar that was never installed.
func (u *Usecase) AwaitAnswer(ctx context.Context, deliveryID string) (HookAnswer, error) {
	slot, held := u.answers.byDeliveryID(deliveryID)
	if !held {
		// Nothing is waiting under that id. It is not an error: the prompt may have
		// been answered between the ingest response and this call, and a relay that
		// asks about a slot that has gone should print nothing and exit, not retry.
		return HookAnswer{}, nil
	}
	timer := time.NewTimer(answerWait(slot.keys.Wait))
	defer timer.Stop()
	select {
	case <-slot.done:
		return HookAnswer{Stdout: slot.stdout}, nil
	case <-timer.C:
		// The budget is deliberately shorter than the provider's own hook timeout, so
		// expiring here is the relay exiting under Crowbar's control rather than being
		// killed mid-write. The prompt stays pending: the human did not answer, but
		// the CLI's dialog is now the thing asking, and the tool-completion sweep will
		// clear the record when the work moves on.
		u.answers.close(slot, nil)
		return HookAnswer{}, nil
	case <-ctx.Done():
		// The relay disconnected — killed, or its own deadline hit first. Free the
		// slot so the prompt cannot be answered into a process that has gone.
		u.answers.close(slot, nil)
		return HookAnswer{}, ctx.Err()
	}
}

// AbandonAnswer is the relay reporting that its prompt was decided somewhere
// else: it was sent SIGTERM, which is the ONLY notification a terminal-side
// DECLINE produces.
//
// Measured against claude 2.1.234: a human who says NO at the PTY fires nothing
// at all — no PostToolUse, no PostToolUseFailure, no PermissionDenied, no Stop.
// The blocked hook is simply killed, and the signal is trappable. Without this
// report the chat would show that prompt as pending until the turn ended, which
// on a declined tool can be a very long time.
//
// A human who says YES needs no report: the gated tool completes, and the
// observation half already resolves the prompt off that completion.
func (u *Usecase) AbandonAnswer(ctx context.Context, deliveryID string) error {
	slot, held := u.answers.byDeliveryID(deliveryID)
	if !held {
		return nil
	}
	u.answers.close(slot, nil)
	u.note(ctx, "choice resolved elsewhere", u.activity.ResolveChoice(
		ctx, slot.chatID, slot.choiceID, domain.ChoiceResolutionProceeded, time.Now(),
	))
	return nil
}

// AnswerChoice records a human's decision and hands it to the relay that is
// holding the provider's gate open.
//
// Order matters. The decision is RENDERED first, so a provider that cannot
// express it is refused before anything is written; then it is recorded, so the
// log has the answer even if the relay dies in the same instant; and only then is
// the relay woken. Waking first would let a chat show an unanswered prompt whose
// answer had already reached the CLI.
func (u *Usecase) AnswerChoice(
	ctx context.Context,
	chatID, choiceID string,
	optionIDs []string,
	reason string,
	content []byte,
) error {
	if _, err := u.chats.GetChat(ctx, chatID); err != nil {
		return fmt.Errorf("agent: answer choice: chat: %w", err)
	}
	slot, held := u.answers.byChoiceID(choiceID)
	if !held || slot.chatID != chatID {
		// No relay is holding this prompt, so there is nowhere for the answer to go:
		// the CLI is showing its own dialog by now, or has moved on. Recording an
		// answer that reached nobody would be the one failure mode this channel exists
		// to prevent.
		return fmt.Errorf("%w: this prompt can no longer be answered from Crowbar",
			apperr.ErrConflict)
	}
	choice, found, err := u.pendingChoice(ctx, chatID, choiceID)
	if err != nil {
		return err
	}
	if !found {
		return fmt.Errorf("%w: this prompt is no longer pending", apperr.ErrConflict)
	}

	decision, err := decide(choice, optionIDs, reason, content)
	if err != nil {
		return err
	}
	if !slot.keys.Accepts(decision.Key) {
		return fmt.Errorf("%w: this provider cannot express that answer", apperr.ErrInvalidArgument)
	}
	agent, err := u.agentForChat(ctx, chatID)
	if err != nil {
		return err
	}
	// Whatever comes back here is a MIS-AUTHORED DESCRIPTOR, not a bad request: the
	// Accepts check above already refused every decision this provider has no
	// template for, so the only failures left are a template that does not render.
	stdout, err := agent.RenderAnswer(slot.event, slot.raw, decision)
	if err != nil {
		return fmt.Errorf("agent: answer choice: render: %w", err)
	}

	if err := u.activity.AnswerChoice(ctx, chatID, choiceID, optionIDs, time.Now()); err != nil {
		return fmt.Errorf("agent: answer choice: %w", err)
	}
	u.answers.close(slot, stdout)
	return nil
}

// decide turns picked option ids into the provider-neutral decision the
// descriptor's templates are rendered against.
//
// The KEY is the chosen option's kind — allow, deny, answer — because that is the
// vocabulary a descriptor declares against. A prompt that offers no options at
// all (an MCP elicitation, whose answer is a form rather than a pick) has no kind
// to read, so the id IS the key: accept, decline, cancel.
//
// WHAT IS PICKED is checked by domain.ActivityChoice.ResolvePicks and by nothing
// here, because the aggregate that records the decision checks it with the same
// call. That rule is what makes a partial answer impossible: the picks must cover
// EVERY question, and an answer that covered one of three is exactly what left a
// live agent saying "still waiting on your answers to questions 2 & 3" with no
// way for anyone to send them.
func decide(
	choice domain.ActivityChoice,
	optionIDs []string,
	reason string,
	content []byte,
) (engineagents.AnswerDecision, error) {
	if len(optionIDs) == 0 {
		return engineagents.AnswerDecision{}, fmt.Errorf(
			"%w: an answer must name at least one option", apperr.ErrInvalidArgument)
	}
	decision := engineagents.AnswerDecision{Reason: reason, Content: content}
	answers, err := choice.ResolvePicks(optionIDs)
	if err != nil {
		return engineagents.AnswerDecision{}, fmt.Errorf("%w: %w", apperr.ErrInvalidArgument, err)
	}
	if len(answers) == 0 {
		// Nothing was offered to pick from, so the id carries the whole decision.
		decision.Key = optionIDs[0]
		return decision, nil
	}

	key, err := decisionKey(answers)
	if err != nil {
		return engineagents.AnswerDecision{}, err
	}
	decision.Key = key
	if key == domain.ChoiceOptionAnswer {
		decision.Answers = answersByQuestion(answers)
	}
	return decision, nil
}

// decisionKey reads the one response template every pick agrees on.
//
// Two picks of different kinds are not one answer: "allow" and "deny" together
// mean nothing, and there is no template that could render them. A question's
// picks are all of kind `answer` by construction, so this only ever bites on a
// permission — which is where it always did.
func decisionKey(answers []domain.ChoiceAnswer) (string, error) {
	key := ""
	for _, answer := range answers {
		for _, option := range answer.Picked {
			if key != "" && key != option.Kind {
				return "", fmt.Errorf(
					"%w: an answer must pick options of one kind", apperr.ErrInvalidArgument)
			}
			key = option.Kind
		}
	}
	return key, nil
}

// answersByQuestion builds the object the provider reads its answers out of: ONE
// KEY PER QUESTION, keyed by the question's own text.
//
// That is what the provider expects to read back — measured against claude
// 2.1.234, an AskUserQuestion is satisfied by an `answers` object mapping each
// question string to the chosen option's label — and it is why a three-question
// prompt must produce three keys. Two keys out of three is not a smaller answer,
// it is an agent left waiting on the third.
//
// A single pick is a bare label and a multi-select's picks are a LIST, per
// question, because that asymmetry is the CLI's own: multiSelect rides each
// question, so one prompt can legitimately produce both shapes at once.
func answersByQuestion(answers []domain.ChoiceAnswer) map[string]any {
	out := make(map[string]any, len(answers))
	for _, answer := range answers {
		key := answer.Question.AnswerKey()
		if key == "" {
			// A question the provider sent neither text nor header for cannot be keyed
			// at all. Skipping it is the only option left, and it is not a partial
			// answer of the kind the coverage rule prevents: there was never a key to
			// file this one under.
			continue
		}
		labels := make([]any, 0, len(answer.Picked))
		for _, option := range answer.Picked {
			labels = append(labels, option.Label)
		}
		// The list shape for a multi-select question, the bare label for a single one
		// — and the list shape for the empty case too, which ResolvePicks has already
		// made unreachable. Indexing labels[0] there would panic a daemon that must
		// never crash on a request, and no answer document is worth that.
		if answer.Question.Multi || len(labels) != 1 {
			out[key] = labels
			continue
		}
		out[key] = labels[0]
	}
	return out
}

func (u *Usecase) pendingChoice(
	ctx context.Context,
	chatID, choiceID string,
) (domain.ActivityChoice, bool, error) {
	choices, err := u.activity.PendingChoices(ctx, chatID)
	if err != nil {
		return domain.ActivityChoice{}, false, fmt.Errorf("agent: answer choice: pending: %w", err)
	}
	for _, choice := range choices {
		if choice.ID == choiceID {
			return choice, true, nil
		}
	}
	return domain.ActivityChoice{}, false, nil
}

// agentForChat resolves the descriptor of the provider whose CLI is on this chat
// right now. It reads the LIVE runner rather than the chat's remembered provider:
// the answer is about to be printed on that process's own hook, so the descriptor
// has to be the one that spawned it.
func (u *Usecase) agentForChat(ctx context.Context, chatID string) (engineagents.Agent, error) {
	runner, err := u.runners.LiveRunnerForChat(ctx, chatID)
	if err != nil {
		return nil, fmt.Errorf("agent: answer choice: runner: %w", err)
	}
	crowbarHome, _, _, _, err := u.ws.WorktreeDir(ctx, runner.WorkspaceID)
	if err != nil {
		return nil, fmt.Errorf("agent: answer choice: worktree dir: %w", err)
	}
	agent, err := u.agents.Get(ctx, crowbarHome, runner.ProviderID)
	if err != nil {
		return nil, fmt.Errorf("agent: answer choice: descriptor: %w", err)
	}
	return agent, nil
}

// AnswerableChoiceIDs reports which of a chat's pending prompts have a relay
// holding their provider's gate open right now.
//
// It is the difference between a question a user can act on and one they can only
// read. A prompt whose relay has timed out is still pending and still worth
// showing — the CLI is asking it — but pressing a button on it would fail, and a
// surface that cannot tell the two apart will offer buttons that do nothing.
func (u *Usecase) AnswerableChoiceIDs(chatID string, choices []domain.ActivityChoice) []string {
	out := make([]string, 0, len(choices))
	for _, choice := range choices {
		if slot, held := u.answers.byChoiceID(choice.ID); held && slot.chatID == chatID {
			out = append(out, choice.ID)
		}
	}
	return out
}

// releaseAnswerWaiters frees every relay a dead CLI left blocked and clears the
// prompts they were holding.
//
// Both halves matter. The relays may be ORPHANS — a killed CLI leaves its hooks
// running (measured against claude 2.1.234) — so they are woken with no verdict
// and exit printing nothing. And the prompts are resolved, because a question
// asked by a process that no longer exists is one nothing else will ever clear.
func (u *Usecase) releaseAnswerWaiters(ctx context.Context, runnerID string) {
	for _, slot := range u.answers.releaseRunner(runnerID) {
		err := u.activity.ResolveChoice(
			ctx, slot.chatID, slot.choiceID, domain.ChoiceResolutionAbandoned, time.Now())
		if err != nil && !errors.Is(err, context.Canceled) {
			slog.WarnContext(ctx, "agent: answer: release prompt of dead runner",
				"runner_id", runnerID, "choice_id", slot.choiceID, "err", err)
		}
	}
}
