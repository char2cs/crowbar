import { afterEach, beforeEach, describe, expect, it } from 'vitest'
import { forgetEntities } from '@/core/entities/entity-store'
import { followWorkspaceScopes } from '@/lib/follow-workspace-scopes'
import { getWorkspaceScope } from '@/lib/workspace-scope'
import { useWorkspaceScopeStore } from '@/lib/workspace-scope-store'
import { putEntities, resetEntities, workspaceDTO } from '@/__tests__/__fixtures__/sidebar-entities'

let stop = () => {}

beforeEach(() => {
  resetEntities()
  useWorkspaceScopeStore.setState(useWorkspaceScopeStore.getInitialState())
})

afterEach(() => stop())

describe('followWorkspaceScopes', () => {
  it('records the scope chat routes are built from, and forgets it when the workspace goes', () => {
    stop = followWorkspaceScopes()
    putEntities('workspace', [
      workspaceDTO({ id: 'ws-home', projectId: 'p2', repoId: '', owningChatId: 'c-home' }),
    ])
    expect(getWorkspaceScope('ws-home')).toMatchObject({
      projectId: 'p2',
      repoId: '',
      owningChatId: 'c-home',
    })

    forgetEntities('workspace', ['ws-home'])
    expect(getWorkspaceScope('ws-home')).toBeNull()
  })

  it('records the workspaces that arrived before it started', () => {
    putEntities('workspace', [workspaceDTO({ id: 'ws1', projectId: 'p1', owningChatId: 'c1' })])
    stop = followWorkspaceScopes()
    expect(getWorkspaceScope('ws1')?.owningChatId).toBe('c1')
  })
})
