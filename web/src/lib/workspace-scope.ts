// §3/§7: the hierarchical scope (owning project+repo) of each workspace, keyed
// by wsId. Lives in this dependency-free module — NOT in workspace-store-registry
// — so the lightweight files/git/lsp/terminal URL builders can resolve it without
// dragging in the heavy editor/Monaco workspace-store graph (which made their
// dynamic-import unit tests time out).
export interface WorkspaceScope {
  projectId: string
  repoId: string
  wsId: string
}

let _activeWorkspaceId: string | null = null
const _scopes = new Map<string, WorkspaceScope>()

/** The wsId of the active workspace route (mirrors the registry's active id). */
export function setActiveScopeWorkspaceId(wsId: string | null): void {
  _activeWorkspaceId = wsId
}

/** Record the hierarchical scope (project+repo) for a workspace from the route. */
export function setWorkspaceScope(scope: WorkspaceScope): void {
  _scopes.set(scope.wsId, scope)
  _activeWorkspaceId = scope.wsId
}

/**
 * Record a workspace's scope WITHOUT making it the active workspace. The
 * sidebar store calls this for every workspace as its data arrives, so actions
 * on a workspace the user never navigated to (e.g. Retry/Detach… on a
 * placeholder row) can still build their scoped URL — workspaceBase throws on
 * an unrecorded scope, which used to make those buttons silently no-op.
 */
export function recordWorkspaceScope(scope: WorkspaceScope): void {
  _scopes.set(scope.wsId, scope)
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
  _activeWorkspaceId = null
}

/**
 * Resolve the hierarchical scope for `wsId` (defaults to the active workspace).
 * Returns null when the scope was never recorded — callers throw/skip so a
 * workspace-scoped URL is never built with a missing project/repo segment.
 */
export function getWorkspaceScope(wsId?: string): WorkspaceScope | null {
  const id = wsId ?? _activeWorkspaceId
  if (!id) return null
  return _scopes.get(id) ?? null
}
