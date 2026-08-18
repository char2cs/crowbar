package handlers

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/char2cs/crowbar/api/internal/api/libs"
	"github.com/char2cs/crowbar/api/internal/api/v0/dto"
)

// maxAnswerReasonBytes bounds the free text a human attaches to a decision. It
// reaches a provider CLI verbatim, so it is capped where it enters rather than
// wherever it is finally printed.
const maxAnswerReasonBytes = 4 << 10

// maxAnswerContentBytes bounds the filled-in form an elicitation is answered
// with. Same reason, and the same shape of cap the requested schema already has.
const maxAnswerContentBytes = 64 << 10

// AnswerChoice handles POST .../agent/chats/:id/choices/:choiceId/answer — a
// human deciding, from Crowbar's chat, a question the provider CLI is blocked on.
//
// This is the whole point of the answer channel: the vendor CLI is sitting on its
// own permission gate waiting for the hook process it fired to exit, and the
// usecase hands that process the decision to print. So the request is a mutation
// with a real failure mode a client must handle — 409 when no relay is holding
// the gate any more, because by then the CLI is asking at its own terminal and
// reporting success would be a lie.
func (h *Handlers) AnswerChoice(ctx *gin.Context) {
	chat, ok := h.requireChatInWorkspace(ctx, ctx.Param("id"))
	if !ok {
		return
	}

	var body struct {
		// OptionIDs name what was picked, by the ids the prompt itself published. A
		// prompt that offers no options — an MCP elicitation, whose answer is a form —
		// takes the provider's own verb here instead: accept, decline, cancel.
		OptionIDs []string `json:"optionIds"`
		// Reason is the human's own words, carried where the provider accepts them.
		Reason string `json:"reason"`
		// Content is a free-form answer document, passed through uninterpreted.
		Content json.RawMessage `json:"content"`
	}
	if err := ctx.ShouldBindJSON(&body); err != nil {
		libs.WriteErr(ctx, http.StatusBadRequest, err.Error())
		return
	}
	if len(body.OptionIDs) == 0 {
		libs.WriteErr(ctx, http.StatusBadRequest, "an answer must name at least one option")
		return
	}
	if len(body.Reason) > maxAnswerReasonBytes {
		libs.WriteErr(ctx, http.StatusBadRequest, "reason is too long")
		return
	}
	if len(body.Content) > maxAnswerContentBytes {
		libs.WriteErr(ctx, http.StatusBadRequest, "content is too long")
		return
	}

	err := h.usecase.AnswerChoice(
		ctx.Request.Context(), chat.ID, ctx.Param("choiceId"),
		body.OptionIDs, body.Reason, body.Content,
	)
	if err != nil {
		status, message := libs.StatusAndMessage(err)
		libs.WriteErr(ctx, status, message)
		return
	}
	libs.WriteMutationOK(ctx, http.StatusOK, ctx.Param("choiceId"))
}

// AwaitHookAnswer handles POST .../agent/hooks/await — the relay parking itself
// until a human answers the prompt its hook just reported.
//
// It is a LONG-POLL, and deliberately not a busy one: the relay is a process the
// vendor CLI is waiting on, so every extra round trip is latency on a gate a user
// is staring at. The daemon holds the request until the prompt is decided or its
// own budget expires, and the request context is what cancels it — a relay that
// dies frees its slot immediately rather than leaving one behind.
//
// It always answers 200. "Nobody decided" is an outcome, not a failure: the relay
// prints nothing, exits 0, and the CLI's own dialog stands — which is exactly
// what a CLI with no Crowbar attached does.
func (h *Handlers) AwaitHookAnswer(ctx *gin.Context) {
	var body struct {
		DeliveryID string `json:"delivery_id"`
	}
	if err := ctx.ShouldBindJSON(&body); err != nil {
		libs.WriteErr(ctx, http.StatusBadRequest, err.Error())
		return
	}
	if body.DeliveryID == "" {
		libs.WriteErr(ctx, http.StatusBadRequest, "delivery_id is required")
		return
	}

	answer, err := h.usecase.AwaitAnswer(ctx.Request.Context(), body.DeliveryID)
	if err != nil {
		if errors.Is(err, ctx.Request.Context().Err()) {
			// The wait was cancelled. This is NOT reliably "the relay hung up":
			// ctx.Request.Context().Err() is non-nil for any cancellation, a
			// server-side deadline included, so this branch can be reached with the
			// relay still listening.
			//
			// So answer it properly rather than returning bare. A bare return writes
			// no body, gin sends 200 with zero bytes, and the relay — which is right
			// to treat a 2xx as an answer — got `unexpected end of JSON input`. An
			// empty Stdout says the same thing in a shape the relay can read: no
			// decision, print nothing, let the CLI's own dialog reach the human.
			// Writing to a genuinely dead connection is harmless.
			libs.WriteQueryOK(ctx, dto.AgentHookAnswerDTO{})
			return
		}
		status, message := libs.StatusAndMessage(err)
		libs.WriteErr(ctx, status, message)
		return
	}
	libs.WriteQueryOK(ctx, dto.AgentHookAnswerDTO{Stdout: string(answer.Stdout)})
}

// AbandonHookAnswer handles POST .../agent/hooks/abandon — the relay reporting
// that its prompt was decided somewhere other than Crowbar.
//
// It is what a trapped SIGTERM does before exiting, and it is the ONLY notice
// Crowbar gets of a terminal-side DECLINE: measured against claude 2.1.234, a
// human who says no at the PTY fires no hook at all — not PostToolUse, not
// PostToolUseFailure, not PermissionDenied, not Stop — and the blocked hook is
// simply killed. Without this the chat would keep showing that question until the
// turn ended.
//
// It must be FAST and it must never fail loudly: the relay is already dying, and
// a hook that hangs while being killed is the one thing this whole path may not
// do.
func (h *Handlers) AbandonHookAnswer(ctx *gin.Context) {
	var body struct {
		DeliveryID string `json:"delivery_id"`
	}
	if err := ctx.ShouldBindJSON(&body); err != nil {
		libs.WriteErr(ctx, http.StatusBadRequest, err.Error())
		return
	}
	if body.DeliveryID == "" {
		libs.WriteErr(ctx, http.StatusBadRequest, "delivery_id is required")
		return
	}
	if err := h.usecase.AbandonAnswer(ctx.Request.Context(), body.DeliveryID); err != nil {
		status, message := libs.StatusAndMessage(err)
		libs.WriteErr(ctx, status, message)
		return
	}
	libs.WriteAccepted(ctx)
}
