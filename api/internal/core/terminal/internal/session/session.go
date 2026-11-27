package session

import (
	"os"
	"os/exec"
	"sync"
	"time"

	"github.com/char2cs/crowbar/api/internal/core/terminal/internal/model"
)

type OutputFrame struct {
	SessionID string
	Data      []byte
	Snapshot  bool
}

type client struct {
	send chan OutputFrame
}

type Session struct {
	id                     string
	ptmx                   *os.File
	cmd                    *exec.Cmd
	model                  model.TerminalModel
	serializer             model.Serializer
	newModel               func(cols, rows, scrollback int) (model.TerminalModel, model.Serializer)
	mu                     sync.Mutex
	clients                map[*client]struct{}
	done                   chan struct{}
	once                   sync.Once
	dirty                  bool
	screenGen              uint64
	exitCode               int
	cwd                    string
	shell                  string
	profileID              string
	command                bool
	lastBlob               []byte
	modelPanics            int
	cols, rows, scrollback int
	replySink              func([]byte)
	lastForegroundPgid     int
	lastFgSampleAt         time.Time
	sampleForegroundLocked func()
	now                    func() time.Time
	pumpNotify             chan struct{}
	emitter                *model.DiffEmitter
	lastEmitAt             time.Time
	emitTimer              *time.Timer

	themeEmitted     bool
	themeEmittedDark bool
}

func (s *Session) ID() string {
	return s.id
}

func (s *Session) Done() <-chan struct{} {
	return s.done
}

func (s *Session) AttachedCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.clients)
}

func (s *Session) CWD() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.cwd
}

func (s *Session) Shell() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.shell
}

func (s *Session) ProfileID() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.profileID
}

func (s *Session) IsCommand() bool {
	return s.command
}

func (s *Session) IsIdle() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.isIdleLocked()
}

func (s *Session) ExitCode() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.exitCode
}
