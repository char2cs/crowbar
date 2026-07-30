package agenttools

// RenderThreadsForTest exposes renderThreads for tests without exporting the
// renderer itself.
var RenderThreadsForTest = renderThreads

// RenderThreadForTest exposes renderThread — the single-thread view — for the
// same reason RenderThreadsForTest exposes the list view.
var RenderThreadForTest = renderThread

// RenderWorkspacesForTest exposes renderWorkspaces for tests without exporting
// the renderer itself.
var RenderWorkspacesForTest = renderWorkspaces

// ChatsByWorkspaceForTest exposes chatsByWorkspace so its membership filter can
// be asserted on the MAP it returns.
//
// The seam exists because the filter is unobservable from list_workspaces'
// output: renderWorkspaces walks the visible slice and looks each id up, so a
// bucket for a workspace the caller cannot see is never reached and the rendered
// text is byte-identical with the filter deleted. A test that can only read the
// rendered text therefore cannot fail when the filter goes — which is exactly
// what happened, and why the assertion has to reach the map.
var ChatsByWorkspaceForTest = chatsByWorkspace

// The result caps, exposed so the bounding tests build fixtures FROM the
// production numbers rather than from copies of them. A test that hardcoded 20
// would keep passing against a cap someone had since changed to 200, which is
// the one way a bounding test can go quiet without anyone noticing.
const (
	DefaultThreadPageForTest   = defaultThreadPage
	MaxThreadPageForTest       = maxThreadPage
	MaxThreadMessagesForTest   = maxThreadMessages
	DefaultChatLogTurnsForTest = defaultChatLogTurns
	MaxChatLogTurnsForTest     = maxChatLogTurns
	DefaultScopeFilesForTest   = defaultScopeFiles
	MaxScopeFilesForTest       = maxScopeFiles
	MaxMessageBodyCharsForTest = maxMessageBodyChars
	MaxTurnBodyCharsForTest    = maxTurnBodyChars
)
