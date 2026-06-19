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
 * Resolve the hierarchical scope for `wsId` (defaults to the active workspace).
 * Returns null when the scope was never recorded — callers throw/skip so a
 * workspace-scoped URL is never built with a missing project/repo segment.
 */
export function getWorkspaceScope(wsId?: string): WorkspaceScope | null {
  const id = wsId ?? _activeWorkspaceId
  if (!id) return null
  return _scopes.get(id) ?? null
}
