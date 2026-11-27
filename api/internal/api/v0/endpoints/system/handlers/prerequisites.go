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

type GitStatus struct {
	Installed bool   `json:"installed"`
	Version   string `json:"version,omitempty"`
}

type CLIStatus struct {
	Installed bool `json:"installed"`
	Authed    bool `json:"authed"`
}

const probeTimeout = 5 * time.Second

const waitDelay = time.Second

type Handler struct {
	probeTimeout time.Duration
}

func NewHandler(_ context.Context) *Handler {
	return &Handler{probeTimeout: probeTimeout}
}

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
