import { describe, it, expect, beforeEach } from 'vitest'
import {
  recordWorkspaceScopeFromPath,
  getWorkspaceScope,
  __resetWorkspaceScopesForTest,
} from '@/lib/workspace-scope'
import { workspaceBase } from '@/lib/workspace-scope-url'

// Regression for the §14 add-repo failure: the workspace scope was recorded
// only in the `$wsId` route component's post-render `useEffect`, but the IDE
// shell renders `WorkspaceView` (whose subtree builds workspace-scoped URLs via
// `workspaceBase`) DURING the same render — before that effect runs. The first
// render therefore threw "no project/repo scope recorded for workspace …" and
// tripped the ErrorBoundary permanently. The route is the canonical scope
// source, so it must be recorded synchronously from the path during render.
beforeEach(() => {
  __resetWorkspaceScopesForTest()
})

describe('recordWorkspaceScopeFromPath', () => {
  it('records the hierarchical scope for an /ide/:p/:r/:w path so workspaceBase resolves synchronously', () => {
    const scope = recordWorkspaceScopeFromPath('/ide/proj-1/repo-2/ws-3')
    expect(scope).toEqual({ projectId: 'proj-1', repoId: 'repo-2', wsId: 'ws-3' })
    // Resolvable immediately — no route effect needed.
    expect(getWorkspaceScope('ws-3')).toEqual({
      projectId: 'proj-1',
      repoId: 'repo-2',
      wsId: 'ws-3',
    })
    expect(workspaceBase('ws-3')).toBe('/v0/projects/proj-1/repos/repo-2/workspaces/ws-3')
  })

  it('captures exactly the three /ide segments, ignoring deeper path parts', () => {
    const scope = recordWorkspaceScopeFromPath('/ide/p/r/w/some/deeper/route')
    expect(scope).toEqual({ projectId: 'p', repoId: 'r', wsId: 'w' })
    expect(workspaceBase('w')).toBe('/v0/projects/p/repos/r/workspaces/w')
  })

  it('makes the recorded workspace the active one (workspaceBase with no arg resolves it)', () => {
    recordWorkspaceScopeFromPath('/ide/p9/r9/w9')
    expect(getWorkspaceScope()).toEqual({ projectId: 'p9', repoId: 'r9', wsId: 'w9' })
  })

  it('returns null and records nothing for a non-ide path', () => {
    expect(recordWorkspaceScopeFromPath('/')).toBeNull()
    expect(recordWorkspaceScopeFromPath('/chat/abc')).toBeNull()
    expect(getWorkspaceScope('w9')).toBeNull()
  })
})
