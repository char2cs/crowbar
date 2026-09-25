# LSP ownership: a session per workspace, owned by its buffers

## Problem

`LspClient` (web) is one singleton that follows the ACTIVE workspace:

- the diagnostics socket (`/v0/chats/<owning chat>/lsp/ws`) is re-pointed at
  whichever workspace becomes active and is never closed — not when its last
  editor goes, not when the workspace is deleted. A deleted workspace's socket
  reconnects against a 404 every 30 s for the life of the page;
- `didOpen`/`didChange`/`didSave`/`didClose` are addressed to the active
  workspace (`wsBase()`), and `useLspDocumentSync` re-runs whenever the active
  workspace's scope readiness flips. A hidden pane of W1 therefore re-sends its
  file, unsaved text included, to W2's language server when W2 becomes active.

There are two copies of "which workspace does this document belong to": the
buffer's own `workspaceId` and the client's `getActiveWorkspaceId()`. They
drift on every workspace switch.

## Owner

The **buffer** owns the workspace of its document. `LspClient` keeps one
`WorkspaceSession` per workspace id, created on demand and addressed only by
the id the caller passes; it never reads the active workspace.

A session holds, for its workspace only: the open-document refcounts, the
opens waiting on the owning chat id, the debounced `didChange`s, the
diagnostics handlers and last batch, and the diagnostics socket.

## Lifecycle (reference-counted)

- A session exists while it has at least one open document or one diagnostics
  handler. The last `documentClose`/handler unsubscribe disposes it: socket
  closed, scope watch dropped, pending changes cancelled.
- The socket is open exactly while the session exists AND the workspace's
  owning chat id is known (`workspace-scope.ts`, the registry every chat-scoped
  URL already resolves through). The session watches that scope: the id
  arriving opens the socket and flushes deferred opens; the id going away
  closes the socket and parks open documents as deferred opens.
- A workspace tombstone (`status: 'deleted'` worktree frame →
  `applyWorkspaceDTO`) and a repo tombstone (the repo's `deleted` frame) now
  **forget** the workspace's scope. That is the existing deleted signal,
  delivered to the one registry LSP already depends on — so the socket closes
  immediately and nothing reconnects against a 404.

## Consequences

- `useLspDocumentSync` needs no readiness gate (`useLspScopeReady` is deleted):
  it opens/closes against the pane's own `workspaceId`, so switching the
  active workspace sends nothing for hidden panes.
- Every document call takes the workspace id: `documentOpen/Close/Save`,
  `scheduleChange`, `flushChange`, `reopen`, `onDiagnosticsUpdate`,
  `onDocumentOpened`. Callers already hold it (buffer, model uri, status menu).
- Daemon: `didOpen` refuses a path that resolves outside the chat's worktree
  (400), so a mis-addressed open can never spawn or feed another root's server.

## Tests (written first)

- W1's open document is not re-sent to W2 when W2 becomes active (hook).
- Two workspaces' documents go to their own chats; last handler/document
  leaving closes that workspace's socket only.
- Forgetting the workspace scope (tombstone) closes its socket immediately and
  it is not reopened.
