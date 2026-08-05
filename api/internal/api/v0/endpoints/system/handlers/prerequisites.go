// Package handlers holds the gin handlers backing the system endpoints.
package handlers

import (
	"bytes"
	"context"
	"os/exec"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/char2cs/crowbar/api/internal/api/libs"
	"github.com/char2cs/crowbar/api/internal/core/binpath"
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

// Handler serves the prerequisite checks. Binary discovery relies on the
// process PATH — which the daemon corrects at startup with the login-shell
// PATH (core/shellenv) — plus binpath.Resolve as the final fallback to
// well-known install dirs.
//
// The previous implementation resolved the login-shell PATH here and placed
// it in cmd.Env, which does NOT work: Go's exec.Command resolves bare binary
// names against the PROCESS PATH at construction time, so the checks failed
// in the packaged app regardless of cmd.Env.
type Handler struct{}

// NewHandler returns a Handler. The context parameter is kept for wiring
// stability; PATH correction now happens process-wide at daemon startup.
func NewHandler(_ context.Context) *Handler {
	return &Handler{}
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

func (h *Handler) checkGit(ctx context.Context) GitStatus {
	var out bytes.Buffer
	//nolint:gosec // G204: fixed git subcommand against a binpath-resolved binary; no user input reaches the command.
	cmd := exec.CommandContext(ctx, binpath.Git(), "--version")
	cmd.Stdout = &out
	if err := cmd.Run(); err != nil {
		return GitStatus{}
	}
	version := strings.TrimPrefix(strings.TrimSpace(out.String()), "git version ")
	return GitStatus{Installed: true, Version: version}
}

func (h *Handler) checkCLI(ctx context.Context, cli string) CLIStatus {
	resolved := binpath.Resolve(cli)
	//nolint:gosec // G204: cli is a fixed literal ("gh"/"glab") resolved to a trusted install path; no user input reaches the command.
	vCmd := exec.CommandContext(ctx, resolved, "--version")
	if vCmd.Run() != nil {
		return CLIStatus{}
	}
	//nolint:gosec // G204: cli is a fixed literal ("gh"/"glab") resolved to a trusted install path; no user input reaches the command.
	aCmd := exec.CommandContext(ctx, resolved, "auth", "status")
	var buf bytes.Buffer
	aCmd.Stdout = &buf
	aCmd.Stderr = &buf
	return CLIStatus{Installed: true, Authed: aCmd.Run() == nil}
}
