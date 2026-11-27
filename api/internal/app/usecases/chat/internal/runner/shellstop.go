package runner

import (
	"context"
	"fmt"

	"github.com/google/uuid"

	"github.com/char2cs/crowbar/api/internal/domain"
	"github.com/char2cs/crowbar/api/internal/domain/ledger"
	"github.com/char2cs/crowbar/api/internal/domain/problem"
)

const shellStopEvent = "shell_stop"

func (rs *Runners) StopShell(ctx context.Context, chatID, shellID string) error {
	st, err := rs.ledger.State(ctx, chatID)
	if err != nil {
		return fmt.Errorf("agent: stop shell: %w", err)
	}
	shell, known := st.ShellOf(shellID)
	if !known || !shell.Live() {
		return fmt.Errorf("agent: stop shell: %q is not a running shell of this chat: %w", shellID, problem.ErrConflict)
	}
	if shell.Stopping {
		return nil
	}
	text, err := rs.shellStopText(ctx, chatID, shellID)
	if err != nil {
		return err
	}
	if _, err := rs.SubmitPrompt(ctx, chatID, text, uuid.NewSHA1(uuid.NameSpaceOID, []byte(chatID+"/"+shellID)).String()); err != nil {
		return fmt.Errorf("agent: stop shell: %w", err)
	}
	_, _, err = rs.ledger.Do(ctx, chatID, func(st ledger.State) ([]ledger.Row, error) {
		shell, known := st.ShellOf(shellID)
		if !known || !shell.Live() || shell.Stopping {
			return nil, nil
		}
		row, err := ledger.NewRow(ledger.KindShell, shell.TurnID, "", ledger.ShellBody{Phase: ledger.ShellStopping, ShellID: shellID})
		return []ledger.Row{row}, err
	})
	if err != nil {
		return fmt.Errorf("agent: stop shell: mark stopping: %w", err)
	}
	return nil
}

func (rs *Runners) shellStopText(ctx context.Context, chatID, shellID string) (string, error) {
	chat, agent, err := rs.chatCapabilityContext(ctx, chatID)
	if err != nil {
		return "", fmt.Errorf("agent: stop shell: %w", err)
	}
	if chat.Surface == domain.SurfaceTerminal {
		return "", ErrShellStopOffSurface
	}
	if _, running := rs.live.byChat(chatID); !running {
		return "", fmt.Errorf("agent: stop shell: the chat has no live agent to ask: %w", problem.ErrConflict)
	}
	wire, payload, ok := agent.OutboundCall(shellStopEvent, map[string]string{"session_id": chat.ID, "shell_id": shellID})
	if !ok {
		return "", fmt.Errorf("agent: stop shell: the provider declares no way to end a shell: %w", problem.ErrNotFound)
	}
	if wire != "prompt" || payload["text"] == "" {
		return "", fmt.Errorf("agent: stop shell: shell_stop must be a prompt with text: %w", problem.ErrInvalidArgument)
	}
	return payload["text"], nil
}
