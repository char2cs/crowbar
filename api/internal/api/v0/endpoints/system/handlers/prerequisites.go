// Package handlers holds the gin handlers backing the system endpoints.
package handlers

import (
	"bytes"
	"context"
	"net/http"
	"os/exec"
	"strings"
	"time"

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

// probeTimeout bounds each probe: a `gh auth status` waiting on a credential helper must
// not hold the first-run screen. A probe that times out reads as absent or not signed in.
const probeTimeout = 5 * time.Second

// waitDelay bounds the wait for a killed probe's output: a shell's child can keep the pipe open.
const waitDelay = time.Second

// Handler serves the prerequisite checks. Binaries resolve against the process PATH, which the
// daemon corrects at startup with the login-shell PATH (exec.Command ignores cmd.Env for lookup),
// with binpath.Resolve falling back to well-known install dirs.
type Handler struct {
	probeTimeout time.Duration
}

// NewHandler returns a Handler. The context is unused: PATH correction happens process-wide at
// daemon startup.
func NewHandler(_ context.Context) *Handler {
	return &Handler{probeTimeout: probeTimeout}
}

// Prerequisite is the GET /system/prerequisites/:tool gin handler: one probe, so a caller
// can show each tool as it answers.
func (h *Handler) Prerequisite(c *gin.Context) {
	ctx := c.Request.Context()
	switch tool := c.Param("tool"); tool {
	case "git":
		libs.WriteQueryOK(c, h.checkGit(ctx))
	case "gh", "glab":
		libs.WriteQueryOK(c, h.checkCLI(ctx, tool))
	default:
		libs.WriteErr(c, http.StatusNotFound, "unknown prerequisite "+tool)
	}
}

// run runs cmd bounded by the probe timeout.
func (h *Handler) run(ctx context.Context, name string, args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, h.probeTimeout)
	defer cancel()
	var out bytes.Buffer
	//nolint:gosec // G204: name is a binpath-resolved fixed tool (git/gh/glab) with fixed args; no user input reaches it.
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Stdout, cmd.Stderr, cmd.WaitDelay = &out, &out, waitDelay
	err := cmd.Run()
	return out.String(), err
}

func (h *Handler) checkGit(ctx context.Context) GitStatus {
	out, err := h.run(ctx, binpath.Git(), "--version")
	if err != nil {
		return GitStatus{}
	}
	version := strings.TrimPrefix(strings.TrimSpace(out), "git version ")
	return GitStatus{Installed: true, Version: version}
}

func (h *Handler) checkCLI(ctx context.Context, cli string) CLIStatus {
	resolved := binpath.Resolve(cli)
	if _, err := h.run(ctx, resolved, "--version"); err != nil {
		return CLIStatus{}
	}
	_, err := h.run(ctx, resolved, "auth", "status")
	return CLIStatus{Installed: true, Authed: err == nil}
}
