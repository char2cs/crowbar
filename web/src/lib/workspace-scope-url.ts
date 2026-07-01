import { getWorkspaceScope } from '@/lib/workspace-scope'

// §3/§7: build the hierarchical base for a workspace-scoped API/WS URL. Every
// files/git/lsp/terminal route nests under the owning project+repo now; callers
// still pass only a wsId (the identifier they hold), and the project/repo are
// resolved from the route-recorded scope (see workspace-store-registry). Throws
// when the scope is unknown so a stale/mis-ordered call fails loudly instead of
// hitting a missing-segment 404.
export function workspaceBase(wsId: string): string {
  const scope = getWorkspaceScope(wsId)
  if (!scope) throw new Error(`no project/repo scope recorded for workspace ${wsId}`)
  const p = encodeURIComponent(scope.projectId)
  // Home workspaces have no repoId — they route to /v0/projects/:p/home/...
  if (!scope.repoId) return `/v0/projects/${p}/home`
  const r = encodeURIComponent(scope.repoId)
  const w = encodeURIComponent(wsId)
  return `/v0/projects/${p}/repos/${r}/workspaces/${w}`
}

// A home (project-level) workspace has no owning repo: its scope is recorded
// with an empty repoId (see the .../home route), which is how workspaceBase
// routes it to /home/... The home workspace deliberately has no git surface on
// the backend, so callers use this to skip git data/streams (files and threads
// ARE home capabilities and stay enabled). Returns false when the scope is
// unknown so non-home workspaces keep their full surface.
export function isHomeWorkspace(wsId: string): boolean {
  return getWorkspaceScope(wsId)?.repoId === ''
}
