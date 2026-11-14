import { describe, expect, it, vi, beforeEach } from 'vitest'
import { stubDropBoundary } from '@/__tests__/__fixtures__/drop-boundary'

import { performSidebarDrop } from '@/components/sidebar/lib/drop-actions'
import { allowedModes } from '@/components/sidebar/lib/sidebar-drop-policy'
import { repoRows } from '@/__tests__/__fixtures__/tree-rows'
import type { Repo } from '@/core/entities/repo-types'
import { __resetWorkspaceScopesForTest } from '@/lib/workspace-scope'
import { resetEntities, seedRepos } from '@/__tests__/__fixtures__/sidebar-entities'
import { toast } from '@/features/window/stores/toast-store'
import * as fetchApi from '@/lib/api/fetch'
import { reparentWorkspace } from '@/lib/api/workspace'

const repo: Repo = {
  id: 'r1',
  projectId: 'p1',
  name: 'repo',
  avatarLabel: 'R',
  avatarColor: 'c',
  defaultWorkspaceId: 'ws-main',
  defaultBranch: 'main',
  defaultOwningChatId: 'main-owner',
  workspaces: [
    {
      id: 'ws-a',
      branch: 'feat/a',
      age: '',
      order: 0,
      status: 'new',
      parentId: 'ws-main',
      owningChatId: '',
    },
    {
      id: 'ws-b',
      branch: 'release',
      age: '',
      order: 1,
      status: 'locked',
      parentId: 'ws-main',
      owningChatId: 'owner-b',
    },
  ],
  folders: [],
  chats: [],
}

beforeEach(() => {
  vi.clearAllMocks()
  stubDropBoundary()
  // The real reparent runs: its chat-keyed address is what refuses a chatless fork.
  vi.mocked(reparentWorkspace).mockRestore()
  vi.spyOn(fetchApi, 'apiFetch').mockResolvedValue(undefined)
  __resetWorkspaceScopesForTest()
  resetEntities()
  seedRepos([repo])
})

// A fork whose owning chat has not been recorded yet is addressed by its
// workspace id; the reparent a nesting drop fires is chat-keyed, so it can
// only throw — the matrix never offers it, and a drop that reaches the
// planner anyway says why instead of surfacing the scope registry error.
describe('dragging a chatless fork into another branch', () => {
  it('is not offered by the drop matrix — only a same-parent reorder is', () => {
    const rows = repoRows(repo)
    const subject = rows.find((r) => r.id === 'ws-a')!
    const target = rows.find((r) => r.id === 'owner-b')!

    expect(allowedModes([subject], target)).toEqual({ before: true, after: true, into: false })
  })

  it('says the chat is still loading instead of surfacing the scope registry error verbatim', async () => {
    const rows = repoRows(repo)
    const subject = rows.find((r) => r.id === 'ws-a')!
    const target = rows.find((r) => r.id === 'owner-b')!

    await performSidebarDrop([subject], target, 'into')

    expect(fetchApi.apiFetch).not.toHaveBeenCalled()
    expect(toast.error).toHaveBeenCalledTimes(1)
    expect(vi.mocked(toast.error).mock.calls[0][0]).toBe(
      "Can't move feat/a yet — its chat is still loading",
    )
  })
})
