package agenttools

// RenderThreadsForTest exposes renderThreads for tests without exporting the
// renderer itself.
var RenderThreadsForTest = renderThreads

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
