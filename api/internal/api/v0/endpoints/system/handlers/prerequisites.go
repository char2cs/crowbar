// Package handlers holds the gin handlers backing the system endpoints.
package handlers

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/char2cs/crowbar/api/internal/api/libs"
)

// GitStatus reports whether the git binary is available and its version string.
type GitStatus struct {
	Installed bool   `json:"installed"`
	Version   string `json:"version,omitempty"`
}

// CLIStatus reports whether a provider CLI is installed and authenticated.
type CLIStatus struct {
	Installed bool `json:"installed"`
	Authed    bool `json:"authed"`
}

// PrerequisitesData is the response payload for GET /system/prerequisites.
type PrerequisitesData struct {
	Git  GitStatus `json:"git"`
	Gh   CLIStatus `json:"gh"`
	Glab CLIStatus `json:"glab"`
}

// Handler holds the startup-resolved environment used for all prerequisite
// checks. Construct it once via NewHandler; do not copy after first use.
type Handler struct {
	// env is os.Environ() with PATH replaced by the login-shell PATH resolved
	// at daemon startup. Falls back to the bare process environment if the
	// shell spawn failed.
	env []string
}

// NewHandler resolves the user's login-shell PATH synchronously and returns a
// Handler ready to serve prerequisite checks. ctx should be the startup
// context (not a request context) so cancellation during a request cannot
// corrupt the cached environment.
func NewHandler(ctx context.Context) *Handler {
	return &Handler{env: loginShellEnv(ctx)}
}

// Prerequisites is the GET /system/prerequisites gin handler.
func (h *Handler) Prerequisites(c *gin.Context) {
	ctx := c.Request.Context()
	libs.WriteQueryOK(c, PrerequisitesData{
		Git:  h.checkGit(ctx),
		Gh:   h.checkCLI(ctx, "gh"),
		Glab: h.checkCLI(ctx, "glab"),
	})
}

// loginShellEnv spawns "$SHELL -l -c 'printf …'" once to obtain the PATH that
// the user's login shell would see, then returns a copy of os.Environ() with
// that PATH substituted in. On failure it returns os.Environ() unchanged so
// system-installed tools (e.g. /usr/bin/git) are still discoverable.
func loginShellEnv(ctx context.Context) []string {
	shell := os.Getenv("SHELL")
	if shell == "" {
		shell = "/bin/sh"
	}

	var stdout, stderr bytes.Buffer
	// stderr is discarded so .zprofile print statements don't pollute stdout.
	cmd := exec.CommandContext(ctx, shell, "-l", "-c", `printf '%s' "$PATH"`)
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	env := os.Environ()
	if err := cmd.Run(); err != nil || stdout.Len() == 0 {
		return env
	}

	loginPath := stdout.String()
	for i, e := range env {
		if strings.HasPrefix(e, "PATH=") {
			env[i] = "PATH=" + loginPath
			return env
		}
	}
	return append(env, "PATH="+loginPath)
}

func (h *Handler) checkGit(ctx context.Context) GitStatus {
	var out bytes.Buffer
	cmd := exec.CommandContext(ctx, "git", "--version")
	cmd.Env = h.env
	cmd.Stdout = &out
	if err := cmd.Run(); err != nil {
		return GitStatus{}
	}
	version := strings.TrimPrefix(strings.TrimSpace(out.String()), "git version ")
	return GitStatus{Installed: true, Version: version}
}

func (h *Handler) checkCLI(ctx context.Context, cli string) CLIStatus {
	vCmd := exec.CommandContext(ctx, cli, "--version")
	vCmd.Env = h.env
	if vCmd.Run() != nil {
		return CLIStatus{}
	}
	aCmd := exec.CommandContext(ctx, cli, "auth", "status")
	aCmd.Env = h.env
	var buf bytes.Buffer
	aCmd.Stdout = &buf
	aCmd.Stderr = &buf
	return CLIStatus{Installed: true, Authed: aCmd.Run() == nil}
}
