package terminal

import (
	"context"
	"errors"
	"image/color"
	"sync"
	"time"

	"github.com/char2cs/crowbar/api/internal/core/terminal/internal/model"
	"github.com/char2cs/crowbar/api/internal/domain"
	"github.com/char2cs/crowbar/api/internal/domain/problem"
)

type WSConn interface {
	WriteMessage(
		messageType int,
		data []byte,
	) error
	ReadMessage() (messageType int, p []byte, err error)
	SetWriteDeadline(t time.Time) error
	Close() error
}

var ErrSessionNotFound = problem.New(problem.NotFound, "terminal: session not found")

var ErrShuttingDown = errors.New("terminal: engine is shutting down")

var ErrCommandNotFound = problem.New(problem.FailedDependency, "terminal: command not found")

var ErrScreenStateUnavailable = errors.New("terminal: screen state unavailable")

type ScreenState = model.ScreenState

type ScreenStateReader interface {
	ScreenState(
		sessionID string,
	) (ScreenState, error)
}

type Exit struct {
	Code   int
	Screen string
}

type Engine interface {
	Create(
		ctx context.Context,
		chatID string,
		workspaceDir string,
		prof *domain.TerminalProfile,
	) (sessionID string, err error)

	CreateCommand(
		ctx context.Context,
		chatID string,
		cwd string,
		argv []string,
		env []string,
		onExit func(Exit),
	) (sessionID string, err error)

	Attach(
		ctx context.Context,
		sessionID string,
		conn WSConn,
	) error

	Write(
		ctx context.Context,
		sessionID string,
		data []byte,
	) error

	Resize(
		ctx context.Context,
		sessionID string,
		cols uint16,
		rows uint16,
	) error

	Kill(
		ctx context.Context,
		sessionID string,
	) error

	TerminateGraceful(
		ctx context.Context,
		sessionID string,
	) error

	Suspend(
		ctx context.Context,
		sessionID string,
	) error

	SetHostTheme(
		bg color.Color,
		fg color.Color,
	)
	HostTheme() (bg, fg color.Color)

	ListSessions() []string

	ListSessionsForChat(
		chatID string,
	) []string

	StateOf(sessionID string) (string, bool)

	SessionExists(
		ctx context.Context,
		sessionID string,
	) bool

	SessionLive(
		ctx context.Context,
		sessionID string,
	) bool

	Screen(
		sessionID string,
		since uint64,
	) (text string, gen uint64, changed bool)

	SetMetaStore(
		s SessionMetaStore,
	)

	LoadPlaceholder(
		ctx context.Context,
		m SessionMeta,
		scrollback []byte,
	) error

	Stats() (active, detached, suspended int, modelBytes int64, degraded, parsePanics int)

	Shutdown()
}

type config struct {
	softLimitPerChat   int
	maxTotalSessions   int
	maxTotalModelBytes int64
	maintenanceTick    time.Duration
	terminateGrace     time.Duration
	writeWait          time.Duration
}

const defaultMaxTotalSessions = 100

func defaultConfig() config {
	return config{
		softLimitPerChat:   10,
		maxTotalSessions:   defaultMaxTotalSessions,
		maxTotalModelBytes: int64(256) << 20,
		maintenanceTick:    10 * time.Second,
		terminateGrace:     3 * time.Second,
		writeWait:          10 * time.Second,
	}
}

func MaxTotalSessions() int {
	return defaultMaxTotalSessions
}

type terminalEngine struct {
	cfg config

	regMu   sync.RWMutex
	entries map[string]*sessionEntry
	byChat  map[string]map[string]*sessionEntry

	cbMu      sync.RWMutex
	metaStore SessionMetaStore

	themeMu          sync.RWMutex
	themeBg, themeFg color.Color

	reaps reapTracker

	stop      chan struct{}
	stopOnce  sync.Once
	maintDone chan struct{}
}

var _ Engine = (*terminalEngine)(nil)

func New() Engine {
	return newEngine(defaultConfig())
}

func newEngine(cfg config) *terminalEngine {
	e := &terminalEngine{
		cfg:       cfg,
		entries:   make(map[string]*sessionEntry),
		byChat:    make(map[string]map[string]*sessionEntry),
		stop:      make(chan struct{}),
		maintDone: make(chan struct{}),
	}
	go e.maintenanceLoop()
	return e
}

func (e *terminalEngine) SetHostTheme(
	bg color.Color,
	fg color.Color,
) {
	e.themeMu.Lock()
	defer e.themeMu.Unlock()
	if bg != nil {
		e.themeBg = bg
	}
	if fg != nil {
		e.themeFg = fg
	}
}

func (e *terminalEngine) HostTheme() (bg, fg color.Color) {
	e.themeMu.RLock()
	defer e.themeMu.RUnlock()
	return e.themeBg, e.themeFg
}

func (e *terminalEngine) SetMetaStore(
	s SessionMetaStore,
) {
	e.cbMu.Lock()
	e.metaStore = s
	e.cbMu.Unlock()
}
