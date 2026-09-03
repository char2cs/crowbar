package profile

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/char2cs/crowbar/api/internal/domain"
)

func TestResolve_NilProfile_UsesEnvShell(t *testing.T) {
	orig := os.Getenv("SHELL")
	t.Setenv("SHELL", "/bin/zsh")
	defer os.Setenv("SHELL", orig)

	r := Resolve(nil, "/workspace")
	assert.Equal(t, "/bin/zsh", r.Shell)
	assert.Equal(t, "/workspace", r.CWD)
	assert.Nil(t, r.Startup)
}

func TestResolve_NilProfile_NoEnvShell_FallsBackToSh(t *testing.T) {
	orig := os.Getenv("SHELL")
	os.Unsetenv("SHELL")
	defer os.Setenv("SHELL", orig)

	r := Resolve(nil, "/ws")
	assert.Equal(t, "/bin/sh", r.Shell)
}

func TestResolve_ProfileShell_Overrides(t *testing.T) {
	t.Setenv("SHELL", "/bin/zsh")
	p := &domain.TerminalProfile{Shell: "/usr/bin/fish"}
	r := Resolve(p, "/ws")
	assert.Equal(t, "/usr/bin/fish", r.Shell)
}

func TestResolve_ProfileCWD_Overrides(t *testing.T) {
	p := &domain.TerminalProfile{StartupDirectory: "/custom/dir"}
	r := Resolve(p, "/workspace")
	assert.Equal(t, "/custom/dir", r.CWD)
}

func TestResolve_WorkspaceDirFallback(t *testing.T) {
	p := &domain.TerminalProfile{}
	r := Resolve(p, "/fallback")
	assert.Equal(t, "/fallback", r.CWD)
}

func TestResolve_StartupCommands(t *testing.T) {
	cmds := []string{"ls -la", "git status"}
	p := &domain.TerminalProfile{StartupCommands: cmds}
	r := Resolve(p, "/ws")
	assert.Equal(t, cmds, r.Startup)
}

func TestResolve_EmptyProfile_Defaults(t *testing.T) {
	orig := os.Getenv("SHELL")
	t.Setenv("SHELL", "/bin/bash")
	defer os.Setenv("SHELL", orig)

	p := &domain.TerminalProfile{}
	r := Resolve(p, "/home/user/project")
	assert.Equal(t, "/bin/bash", r.Shell)
	assert.Equal(t, "/home/user/project", r.CWD)
	assert.Nil(t, r.Startup)
}
