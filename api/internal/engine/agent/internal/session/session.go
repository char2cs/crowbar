// Package session implements the AgentRuntime via the ACP SDK subprocess protocol.
package session

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"

	acp "github.com/coder/acp-go-sdk"
	"github.com/google/uuid"

	"github.com/char2cs/crowbar/api/internal/core/config"
	"github.com/char2cs/crowbar/api/internal/domain"
	"github.com/char2cs/crowbar/api/internal/engine/agent/internal/artifacts"
	"github.com/char2cs/crowbar/api/internal/engine/agent/internal/drain"
	"github.com/char2cs/crowbar/api/internal/engine/agent/internal/prompt"
	"github.com/char2cs/crowbar/api/internal/engine/agent/internal/resolver"
	"github.com/char2cs/crowbar/api/internal/engine/agent/internal/types"
	"github.com/char2cs/crowbar/api/internal/engine/flow"
)

// Hub registers a chat session for a task and returns the input channel, the
// publish function, and an unregister cleanup.
type Hub interface {
	RegisterSession(taskID string) (input <-chan string, publish func(types.ChatFrame), unregister func())
}

// Runtime implements agent.AgentRuntime using ACP subprocess spawning.
type Runtime struct {
	cfg         config.IntelligenceConfig
	runsDirFn   func(runID string) string
	hub         Hub
	mcpURL      string
	agentCfgDir string
}

// New constructs a Runtime. runsDirFn returns the path of the run-specific
// directory for the given run ID. mcpURL, when non-empty, is passed to the ACP
// session so the subprocess can reach the crowbar MCP endpoint. agentCfgDir,
// when non-empty, is exported to the agent subprocess as CLAUDE_CONFIG_DIR so
// it runs with an isolated configuration rather than the host user's ~/.claude.
func New(
	cfg config.IntelligenceConfig,
	runsDirFn func(runID string) string,
	hub Hub,
	mcpURL string,
	agentCfgDir string,
) *Runtime {
	return &Runtime{
		cfg:         cfg,
		runsDirFn:   runsDirFn,
		hub:         hub,
		mcpURL:      mcpURL,
		agentCfgDir: agentCfgDir,
	}
}

// Run executes a single agent state within an ACP subprocess.
func (r *Runtime) Run(
	ctx context.Context,
	run domain.AgentRun,
	task domain.Task,
	state flow.StateDefinition,
) error {
	if state.Agent == nil {
		return fmt.Errorf("session: state %q has no agent definition", state.Name)
	}

	runsDir := r.runsDirFn(run.ID)
	if err := os.MkdirAll(runsDir, 0o755); err != nil {
		return fmt.Errorf("session: mkdir %s: %w", runsDir, err)
	}

	eventsPath := filepath.Join(runsDir, "events.jsonl")
	eventsFile, err := os.Create(eventsPath)
	if err != nil {
		return fmt.Errorf("session: create events.jsonl: %w", err)
	}
	defer eventsFile.Close()

	bin, args, err := resolver.Resolve(state.Agent.Intelligence, r.cfg)
	if err != nil {
		return fmt.Errorf("session: resolve agent: %w", err)
	}

	if r.agentCfgDir != "" {
		if err := os.MkdirAll(r.agentCfgDir, 0o755); err != nil {
			return fmt.Errorf("session: mkdir agent config dir %s: %w", r.agentCfgDir, err)
		}
	}

	cmd := exec.CommandContext(ctx, bin, args...)
	cmd.Dir = task.WorktreePath
	cmd.Env = agentEnv(r.agentCfgDir)

	stdin, err := cmd.StdinPipe()
	if err != nil {
		return fmt.Errorf("session: stdin pipe: %w", err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("session: stdout pipe: %w", err)
	}

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("session: start %s: %w", bin, err)
	}
	defer cmd.Process.Kill() //nolint:errcheck

	sc := newSessionClient()
	conn := acp.NewClientSideConnection(sc, stdin, stdout)

	_, err = conn.Initialize(ctx, acp.InitializeRequest{
		ProtocolVersion: acp.ProtocolVersionNumber,
		ClientCapabilities: acp.ClientCapabilities{
			Fs:       acp.FileSystemCapabilities{ReadTextFile: true, WriteTextFile: true},
			Terminal: true,
		},
	})
	if err != nil {
		return fmt.Errorf("session: initialize: %w", err)
	}

	var mcpServers []acp.McpServer
	if r.mcpURL != "" {
		mcpServers = []acp.McpServer{{
			Http: &acp.McpServerHttpInline{
				Name: "crowbar",
				Type: "http",
				Url:  r.mcpURL,
				Headers: []acp.HttpHeader{{
					Name:  "X-Agent-Run-Token",
					Value: run.Token,
				}},
			},
		}}
	}

	sess, err := conn.NewSession(ctx, acp.NewSessionRequest{
		Cwd:        task.WorktreePath,
		McpServers: mcpServers,
	})
	if err != nil {
		return fmt.Errorf("session: new session: %w", err)
	}

	priors := toPriorOutputs(run.Outputs)
	promptText := prompt.Build(state, task.Title, priors)

	_, err = conn.Prompt(ctx, acp.PromptRequest{
		SessionId: sess.SessionId,
		Prompt:    []acp.ContentBlock{acp.TextBlock(promptText)},
	})
	if err != nil {
		return fmt.Errorf("session: prompt: %w", err)
	}

	_, publish, unregister := r.hub.RegisterSession(task.ID)
	defer unregister()

	drainCtx, drainCancel := context.WithCancel(ctx)
	defer drainCancel()

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		_ = drain.Run(drainCtx, sc.updates(), eventsFile, publish)
	}()

	select {
	case <-conn.Done():
	case <-ctx.Done():
	}

	drainCancel()
	wg.Wait()

	diffPath := filepath.Join(runsDir, "diff.patch")
	_ = artifacts.WriteDiff(task.WorktreePath, task.BaseBranch, task.BranchName, diffPath)

	return nil
}

// agentEnv returns the environment for the agent subprocess. It inherits the
// parent environment but:
//
//   - Strips variables that mark an active Claude Code session. The
//     claude-code-acp bridge refuses to launch when CLAUDECODE is set ("Claude
//     Code cannot be launched inside another Claude Code session"), so the
//     crowbar server process must not propagate that marker to its agents.
//   - Sets CLAUDE_CONFIG_DIR to cfgDir when non-empty, so the agent runs with an
//     isolated configuration (no host user skills, settings, or CLAUDE.md
//     memory). This keeps agent behaviour deterministic regardless of the
//     machine crowbar runs on.
func agentEnv(cfgDir string) []string {
	stripped := map[string]struct{}{
		"CLAUDECODE":             {},
		"CLAUDE_CODE_ENTRYPOINT": {},
		"CLAUDE_CODE_SSE_PORT":   {},
		"CLAUDE_CONFIG_DIR":      {},
	}
	parent := os.Environ()
	out := make([]string, 0, len(parent)+1)
	for _, kv := range parent {
		name := kv
		if i := strings.IndexByte(kv, '='); i >= 0 {
			name = kv[:i]
		}
		if _, drop := stripped[name]; drop {
			continue
		}
		out = append(out, kv)
	}
	if cfgDir != "" {
		out = append(out, "CLAUDE_CONFIG_DIR="+cfgDir)
	}
	return out
}

func toPriorOutputs(
	outputs []domain.AgentRunOutput,
) []prompt.AgentRunOutput {
	if len(outputs) == 0 {
		return nil
	}
	result := make([]prompt.AgentRunOutput, len(outputs))
	for i, o := range outputs {
		result[i] = prompt.AgentRunOutput{
			StateName: o.StateName,
			Output:    o.Output,
		}
	}
	return result
}

// terminalEntry tracks a running ACP terminal subprocess.
type terminalEntry struct {
	cancel     context.CancelFunc
	cmd        *exec.Cmd
	buf        bytes.Buffer
	mu         sync.Mutex
	done       chan struct{}
	truncated  bool
	exitCode   *int
	exitSignal *string
	limit      int
}

// termWriter writes to a terminalEntry buffer under the entry mutex, truncating
// from the beginning when the output byte limit is exceeded.
type termWriter struct{ e *terminalEntry }

func (w *termWriter) Write(p []byte) (int, error) {
	e := w.e
	e.mu.Lock()
	defer e.mu.Unlock()
	e.buf.Write(p)
	if e.limit > 0 && e.buf.Len() > e.limit {
		excess := e.buf.Len() - e.limit
		tail := make([]byte, e.limit)
		copy(tail, e.buf.Bytes()[excess:])
		e.buf.Reset()
		e.buf.Write(tail)
		e.truncated = true
	}
	return len(p), nil
}

// sessionClient implements acp.Client and routes SessionUpdate notifications
// to an internal channel for the drain goroutine.
type sessionClient struct {
	mu        sync.Mutex
	ch        chan acp.SessionNotification
	rootDir   string
	terminals sync.Map // string → *terminalEntry
}

func newSessionClient() *sessionClient {
	return &sessionClient{
		ch: make(chan acp.SessionNotification, 64),
	}
}

func (c *sessionClient) updates() <-chan acp.SessionNotification {
	return c.ch
}

func (c *sessionClient) SessionUpdate(
	_ context.Context,
	n acp.SessionNotification,
) error {
	select {
	case c.ch <- n:
	default:
	}
	return nil
}

func (c *sessionClient) RequestPermission(
	_ context.Context,
	p acp.RequestPermissionRequest,
) (acp.RequestPermissionResponse, error) {
	if len(p.Options) == 0 {
		return acp.RequestPermissionResponse{
			Outcome: acp.RequestPermissionOutcome{
				Cancelled: &acp.RequestPermissionOutcomeCancelled{},
			},
		}, nil
	}
	return acp.RequestPermissionResponse{
		Outcome: acp.RequestPermissionOutcome{
			Selected: &acp.RequestPermissionOutcomeSelected{OptionId: p.Options[0].OptionId},
		},
	}, nil
}

func (c *sessionClient) ReadTextFile(
	_ context.Context,
	p acp.ReadTextFileRequest,
) (acp.ReadTextFileResponse, error) {
	data, err := os.ReadFile(p.Path)
	if err != nil {
		return acp.ReadTextFileResponse{}, err
	}
	content := string(data)
	if p.Line != nil || p.Limit != nil {
		lines := strings.Split(content, "\n")
		start := 0
		if p.Line != nil && *p.Line > 0 && *p.Line-1 < len(lines) {
			start = *p.Line - 1
		}
		end := len(lines)
		if p.Limit != nil && *p.Limit > 0 && start+*p.Limit < end {
			end = start + *p.Limit
		}
		content = strings.Join(lines[start:end], "\n")
	}
	return acp.ReadTextFileResponse{Content: content}, nil
}

func (c *sessionClient) WriteTextFile(
	_ context.Context,
	p acp.WriteTextFileRequest,
) (acp.WriteTextFileResponse, error) {
	if dir := filepath.Dir(p.Path); dir != "" {
		_ = os.MkdirAll(dir, 0o755)
	}
	return acp.WriteTextFileResponse{}, os.WriteFile(p.Path, []byte(p.Content), 0o644)
}

func (c *sessionClient) CreateTerminal(
	_ context.Context,
	req acp.CreateTerminalRequest,
) (acp.CreateTerminalResponse, error) {
	const defaultLimit = 1 << 20 // 1 MiB
	limit := defaultLimit
	if req.OutputByteLimit != nil && *req.OutputByteLimit > 0 {
		limit = *req.OutputByteLimit
	}

	termCtx, cancel := context.WithCancel(context.Background())
	entry := &terminalEntry{
		cancel: cancel,
		done:   make(chan struct{}),
		limit:  limit,
	}

	cmd := exec.CommandContext(termCtx, req.Command, req.Args...)
	if req.Cwd != nil {
		cmd.Dir = *req.Cwd
	}
	if len(req.Env) > 0 {
		env := os.Environ()
		for _, e := range req.Env {
			env = append(env, e.Name+"="+e.Value)
		}
		cmd.Env = env
	}
	w := &termWriter{e: entry}
	cmd.Stdout = w
	cmd.Stderr = w
	entry.cmd = cmd

	if err := cmd.Start(); err != nil {
		cancel()
		return acp.CreateTerminalResponse{}, fmt.Errorf("terminal: start %q: %w", req.Command, err)
	}

	go func() {
		defer close(entry.done)
		err := cmd.Wait()
		entry.mu.Lock()
		defer entry.mu.Unlock()
		if err != nil {
			var exitErr *exec.ExitError
			if errors.As(err, &exitErr) {
				code := exitErr.ExitCode()
				if code >= 0 {
					entry.exitCode = &code
				} else {
					sig := exitErr.ProcessState.String()
					entry.exitSignal = &sig
				}
			}
		} else {
			code := 0
			entry.exitCode = &code
		}
	}()

	id := uuid.NewString()
	c.terminals.Store(id, entry)
	return acp.CreateTerminalResponse{TerminalId: id}, nil
}

func (c *sessionClient) KillTerminal(
	_ context.Context,
	req acp.KillTerminalRequest,
) (acp.KillTerminalResponse, error) {
	v, ok := c.terminals.Load(req.TerminalId)
	if !ok {
		return acp.KillTerminalResponse{}, fmt.Errorf("terminal: unknown id %q", req.TerminalId)
	}
	v.(*terminalEntry).cancel()
	return acp.KillTerminalResponse{}, nil
}

func (c *sessionClient) ReleaseTerminal(
	_ context.Context,
	req acp.ReleaseTerminalRequest,
) (acp.ReleaseTerminalResponse, error) {
	v, ok := c.terminals.LoadAndDelete(req.TerminalId)
	if ok {
		v.(*terminalEntry).cancel()
	}
	return acp.ReleaseTerminalResponse{}, nil
}

func (c *sessionClient) TerminalOutput(
	_ context.Context,
	req acp.TerminalOutputRequest,
) (acp.TerminalOutputResponse, error) {
	v, ok := c.terminals.Load(req.TerminalId)
	if !ok {
		return acp.TerminalOutputResponse{}, fmt.Errorf("terminal: unknown id %q", req.TerminalId)
	}
	e := v.(*terminalEntry)
	e.mu.Lock()
	output := e.buf.String()
	truncated := e.truncated
	exitCode := e.exitCode
	exitSignal := e.exitSignal
	e.mu.Unlock()

	if output == "" {
		output = " " // ACP requires non-empty output
	}
	resp := acp.TerminalOutputResponse{Output: output, Truncated: truncated}
	select {
	case <-e.done:
		resp.ExitStatus = &acp.TerminalExitStatus{ExitCode: exitCode, Signal: exitSignal}
	default:
	}
	return resp, nil
}

func (c *sessionClient) WaitForTerminalExit(
	ctx context.Context,
	req acp.WaitForTerminalExitRequest,
) (acp.WaitForTerminalExitResponse, error) {
	v, ok := c.terminals.Load(req.TerminalId)
	if !ok {
		return acp.WaitForTerminalExitResponse{}, fmt.Errorf("terminal: unknown id %q", req.TerminalId)
	}
	e := v.(*terminalEntry)
	select {
	case <-e.done:
	case <-ctx.Done():
		return acp.WaitForTerminalExitResponse{}, ctx.Err()
	}
	e.mu.Lock()
	exitCode := e.exitCode
	exitSignal := e.exitSignal
	e.mu.Unlock()
	return acp.WaitForTerminalExitResponse{ExitCode: exitCode, Signal: exitSignal}, nil
}
