//go:build integration && unix

package scripted_test

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// hookCLI is a vendor TUI that reports over hook commands: claude always,
// codex in a terminal. The two differ only in how hooks are configured, where
// sessions live and when the session is announced.
type hookCLI struct {
	name    string
	hooks   map[string][]string
	cwd     string
	session string
	// sessionFile is where this vendor keeps a session's transcript.
	sessionFile func(id string) string
	// lazySession announces the session on the first turn, not at boot
	// (codex 0.156 fires SessionStart lazily).
	lazySession bool
	announced   bool
}

// runHookCLI boots the TUI: refuse or accept the resume, announce, run the
// launch prompt, then take every line typed into the PTY as a prompt.
func runHookCLI(c *hookCLI, resumeID, prompt string) int {
	record(c.name, "start", map[string]any{"argv": os.Args[1:], "resume": resumeID, "prompt": prompt})
	c.session = newID()
	if resumeID != "" {
		if loadScript().Resume == "refuse" || !exists(c.sessionFile(resumeID)) {
			record(c.name, "refused", map[string]any{"session": resumeID})
			say("No conversation found with session ID: " + resumeID + "\n")
			return 1
		}
		c.session = resumeID
	}
	say(c.name + " ready\n> ")
	if !c.lazySession {
		c.announce("startup")
	}
	if prompt != "" {
		c.turn(prompt)
	}
	lines := bufio.NewScanner(os.Stdin)
	for lines.Scan() {
		if typed := strings.TrimSpace(lines.Text()); typed != "" {
			c.turn(typed)
		}
	}
	hang()
	return 0
}

func (c *hookCLI) announce(source string) {
	c.announced = true
	c.fire("SessionStart", map[string]any{"source": source})
}

func (c *hookCLI) base(event string) map[string]any {
	return map[string]any{
		"session_id": c.session, "transcript_path": c.sessionFile(c.session),
		"cwd": c.cwd, "hook_event_name": event,
	}
}

// fire runs every command wired to event and returns the last one's stdout.
func (c *hookCLI) fire(event string, fields map[string]any) string {
	payload := c.base(event)
	for k, v := range fields {
		payload[k] = v
	}
	out := ""
	for _, command := range c.hooks[event] {
		out = runHook(command, payload)
	}
	return out
}

func (c *hookCLI) turn(prompt string) {
	s := loadScript()
	if !c.announced {
		c.announce("startup")
	}
	record(c.name, "prompt", map[string]any{"text": prompt, "session": c.session})
	writeSessionFile(c.sessionFile(c.session))
	if strings.TrimSpace(prompt) == "/compact" {
		// A built-in: the CLI compacts and answers nothing.
		c.fire("PreCompact", map[string]any{"trigger": "manual"})
		c.fire("PostCompact", map[string]any{"trigger": "manual"})
		return
	}
	c.fire("UserPromptSubmit", map[string]any{"prompt": prompt})
	for i, st := range s.Turn {
		c.step(i, st)
	}
	c.fire("Stop", map[string]any{"last_assistant_message": s.reply(), "background_tasks": []any{}})
}

func (c *hookCLI) step(i int, st step) {
	id := c.session[:8] + "-" + string(rune('a'+i))
	switch {
	case st.Tool != "":
		input := map[string]any{"command": "echo " + id}
		c.fire("PreToolUse", map[string]any{"tool_name": st.Tool, "tool_input": input, "tool_use_id": id})
		c.fire("PostToolUse", map[string]any{
			"tool_name": st.Tool, "tool_input": input, "tool_use_id": id, "tool_response": id, "duration_ms": 1,
		})
	case st.Subagent != "":
		c.fire("SubagentStart", map[string]any{"agent_id": id, "agent_type": st.Subagent})
		c.fire("SubagentStop", map[string]any{"agent_id": id, "agent_type": st.Subagent})
	case st.Permission != "":
		verdict := c.fire("PermissionRequest", map[string]any{
			"tool_name": st.Permission, "tool_input": map[string]any{"command": "rm -rf " + id},
		})
		record(c.name, "answer", map[string]any{"verdict": verdict})
	case st.Compact:
		c.fire("PreCompact", map[string]any{"trigger": "auto"})
		c.fire("PostCompact", map[string]any{"trigger": "auto"})
	case st.Hang:
		hang()
	case st.Crash:
		record(c.name, "crash", nil)
		os.Exit(3)
	case st.SlowMS > 0:
		pause(st.SlowMS)
	}
}

func exists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// runClaude is claude's TUI: hooks from --settings, resume by --resume, the
// prompt after "--", transcripts under CLAUDE_CONFIG_DIR/projects.
func runClaude(args []string) int {
	switch {
	case hasArg(args, "--version"):
		say("2.1.281 (Claude Code)\n")
		return 0
	case len(args) > 0 && args[0] == "plugin":
		say("[]\n")
		return 0
	}
	cwd, _ := os.Getwd()
	c := &hookCLI{
		name: "claude", hooks: claudeHooks(argAfter(args, "--settings")), cwd: cwd,
		sessionFile: func(id string) string {
			slug := strings.ReplaceAll(cwd, string(filepath.Separator), "-")
			return filepath.Join(os.Getenv("CLAUDE_CONFIG_DIR"), "projects", slug, id+".jsonl")
		},
	}
	return runHookCLI(c, argAfter(args, "--resume"), positionalsAfterDashes(args))
}

func claudeHooks(settingsPath string) map[string][]string {
	raw, err := os.ReadFile(settingsPath)
	if err != nil {
		return nil
	}
	var settings struct {
		Hooks map[string][]struct {
			Hooks []struct {
				Command string `json:"command"`
			} `json:"hooks"`
		} `json:"hooks"`
	}
	if json.Unmarshal(raw, &settings) != nil {
		return nil
	}
	out := map[string][]string{}
	for event, matchers := range settings.Hooks {
		for _, m := range matchers {
			for _, h := range m.Hooks {
				out[event] = append(out[event], h.Command)
			}
		}
	}
	return out
}

var codexHookRE = regexp.MustCompile(`^hooks\.([A-Za-z]+)=.*command="([^"]*)"`)

// codexHooks reads codex's `-c hooks.<Event>=[{hooks=[{command="…"}]}]` wiring.
func codexHooks(args []string) map[string][]string {
	out := map[string][]string{}
	for i := 0; i+1 < len(args); i++ {
		if args[i] != "-c" {
			continue
		}
		if m := codexHookRE.FindStringSubmatch(args[i+1]); m != nil {
			out[m[1]] = append(out[m[1]], m[2])
		}
	}
	return out
}
