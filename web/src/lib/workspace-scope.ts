import { useSyncExternalStore } from 'react'

// §3/§7: the hierarchical scope (owning project+repo) of each workspace, keyed
// by wsId. Lives in this dependency-free module — NOT in workspace-store-registry
// — so the lightweight files/git/lsp/terminal URL builders can resolve it without
// dragging in the heavy editor/Monaco workspace-store graph (which made their
// dynamic-import unit tests time out).
export interface WorkspaceScope {
  projectId: string
  repoId: string
  wsId: string
  /**
   * The CHAT that owns this workspace's worktree, straight from the daemon
   * (`WorkspaceDTO.owningChatId`) — never guessed here.
   *
   * It is what chat-scoped API routes are addressed by (`/v0/chats/:chatId/...`),
   * so a caller holding only a wsId can still reach them. Optional because the
   * ROUTE (/ide/:projectId/:repoId/:wsId) cannot supply it — only the sidebar's
   * workspace data can — which is why setWorkspaceScope below merges rather
   * than overwrites it.
   */
  owningChatId?: string
}

/** Reads THE active workspace id, which the workspace registry owns (C4). */
let readActiveWorkspaceId: () => string | null = () => null
const _scopes = new Map<string, WorkspaceScope>()

// Notified whenever a workspace's scope is (re)written — the only signal a
// caller has that `getOwningChatId(wsId)` might now answer differently.
// `_scopes` is a plain Map (not a store) precisely to stay dependency-free;
// this is the minimal addition that lets `useOwningChatId` (use-workspace-
// effects.ts) treat "the sidebar hasn't recorded an owning chat yet" as a
// state to wait on and re-render for, instead of a one-shot answer read once
// at mount. See `subscribeToWorkspaceScope` below.
const _scopeListeners = new Map<string, Set<() => void>>()

function notifyScopeListeners(wsId: string): void {
  const listeners = _scopeListeners.get(wsId)
  if (!listeners) return
  for (const listener of listeners) listener()
}

/**
 * Subscribe to every future write of `wsId`'s scope (route-derived or
 * sidebar-derived). Fires on EVERY write, not just ones that change
 * `owningChatId` — callers that only care about that field re-read it
 * themselves and no-op if it hasn't actually changed, and writes are rare
 * enough (once per navigation, once per chat-list refresh) that this stays
 * cheap without the extra bookkeeping a diff would need.
 */
export function subscribeToWorkspaceScope(wsId: string, callback: () => void): () => void {
  let listeners = _scopeListeners.get(wsId)
  if (!listeners) {
    listeners = new Set()
    _scopeListeners.set(wsId, listeners)
  }
  listeners.add(callback)
  return () => {
    listeners!.delete(callback)
    if (listeners!.size === 0) _scopeListeners.delete(wsId)
  }
}

/** Bind the owner of the active workspace id (the registry) — this module
 *  stays dependency-free and keeps no copy of it. */
export function bindActiveWorkspaceId(read: () => string | null): void {
  readActiveWorkspaceId = read
}

/**
 * Merge a scope into the registry, PRESERVING a previously recorded
 * owningChatId when the incoming scope carries none.
 *
 * The route parser and the sidebar both write here, and only the sidebar knows
 * the owning chat. Without this merge, navigating to a workspace (a route-derived
 * write with no chat) would erase the chat id the sidebar had already recorded,
 * and every chat-scoped URL for that workspace would start throwing.
 */
function mergeScope(scope: WorkspaceScope): WorkspaceScope {
  const prev = _scopes.get(scope.wsId)
  const owningChatId = scope.owningChatId || prev?.owningChatId
  return owningChatId ? { ...scope, owningChatId } : { ...scope }
}

/** Record the hierarchical scope (project+repo) for a workspace from the
 *  route. It never decides which workspace is active: that is the registry's
 *  one id, written by `WorkspaceHost`. */
export function setWorkspaceScope(scope: WorkspaceScope): void {
  _scopes.set(scope.wsId, mergeScope(scope))
  notifyScopeListeners(scope.wsId)
}

/**
 * Record a workspace's scope WITHOUT making it the active workspace. The
 * sidebar store calls this for every workspace as its data arrives, so actions
 * on a workspace the user never navigated to (e.g. Retry/Detach… on a
 * placeholder row) can still build their scoped URL — workspaceBase throws on
 * an unrecorded scope, which used to make those buttons silently no-op.
 */
export function recordWorkspaceScope(scope: WorkspaceScope): void {
  _scopes.set(scope.wsId, mergeScope(scope))
  notifyScopeListeners(scope.wsId)
}

// The router pathname for the active workspace route. Not anchored to the start
// so it also matches the hash-history in-hash path; captures exactly the three
// /ide/:projectId/:repoId/:wsId segments and stops at the next separator.
const IDE_ROUTE = /\/ide\/([^/]+)\/([^/]+)\/([^/]+)/

/**
 * Parse an /ide/:projectId/:repoId/:wsId pathname and record its scope, then
 * return it (or null for a non-/ide path). The IDE shell calls this SYNCHRONOUSLY
 * during render — before it renders WorkspaceView — because the workspace panels
 * build workspace-scoped URLs (workspaceBase) during their own render. Recording
 * the scope only in the route component's post-render effect was too late: the
 * first render threw and tripped the ErrorBoundary. The route is the canonical
 * scope source, so deriving it here keeps the lookup resolvable on first paint.
 */
export function recordWorkspaceScopeFromPath(pathname: string): WorkspaceScope | null {
  const scope = parseWorkspaceScopeFromPath(pathname)
  if (scope) setWorkspaceScope(scope)
  return scope
}

/**
 * Pure parse of an /ide/:projectId/:repoId/:wsId pathname into its scope, WITHOUT
 * recording it (no side effect). Returns null for a non-/ide path. Render paths
 * that only need to READ the active workspace from the route — e.g. the context
 * pill — use this so they react to pathname changes without mutating the registry
 * (which the IDE shell owns). The route shape is /ide/:p/:r/:wsId; matching the
 * legacy /workspaces/:wsId shape here is the bug this replaced — the pill then
 * never resolved a workspace and always showed the project name.
 */
export function parseWorkspaceScopeFromPath(pathname: string): WorkspaceScope | null {
  const match = pathname.match(IDE_ROUTE)
  if (!match) return null
  return { projectId: match[1], repoId: match[2], wsId: match[3] }
}

/** Testing only — clears the in-memory scope registry between tests. */
export function __resetWorkspaceScopesForTest(): void {
  _scopes.clear()
  _scopeListeners.clear()
}

/**
 * Resolve the hierarchical scope for `wsId` (defaults to the active workspace).
 * Returns null when the scope was never recorded — callers throw/skip so a
 * workspace-scoped URL is never built with a missing project/repo segment.
 */
export function getWorkspaceScope(wsId?: string): WorkspaceScope | null {
  const id = wsId ?? readActiveWorkspaceId()
  if (!id) return null
  return _scopes.get(id) ?? null
}

/**
 * The chat that owns `wsId`'s worktree (defaults to the active workspace), or
 * null when the scope was never recorded or the daemon resolved no owning chat.
 *
 * This is the bridge from "the id a terminal component holds" (a workspace) to
 * "the id its API routes are addressed by" (a chat). Callers throw or skip on
 * null rather than falling back to a workspace-scoped URL — those routes no
 * longer exist.
 */
export function getOwningChatId(wsId?: string): string | null {
  return getWorkspaceScope(wsId)?.owningChatId || null
}

/**
 * Whether `wsId`'s project/repo scope has been recorded yet — `workspaceBase`
 * (and anything built on it, e.g. `repoChatsBaseForWorkspace`) throws without
 * it. WorkspaceHost can force-mount a workspace's effects (a pane/Recents-
 * retained workspace nobody has navigated to yet) before the route or the
 * sidebar's own repo fetch has recorded its scope — most reliably right after
 * a cold boot, when which of the two finishes first is a genuine race. This
 * makes readiness a piece of React state a caller can wait on, and re-fire
 * once it resolves, instead of calling straight into the throw.
 *
 * For a caller that only needs the OWNING CHAT specifically (chat-scoped
 * routes — files/git/lsp/terminal), use `getOwningChatId` with this same
 * `subscribeToWorkspaceScope` wiring instead (see `useOwningChatId`,
 * use-workspace-effects.ts) — scope can be recorded (route-derived, no chat
 * yet) well before an owning chat is, so the two readiness questions are
 * genuinely different.
 */
export function useWorkspaceScopeReady(wsId: string): boolean {
  return useSyncExternalStore(
    (onChange) => subscribeToWorkspaceScope(wsId, onChange),
    () => getWorkspaceScope(wsId) !== null,
  )
}
