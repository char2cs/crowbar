import { describe, it, expect, beforeEach } from 'vitest'
import { upsertEntity } from '@/lib/persistence/entity-cache'
import { wipeEntityCache } from '@/lib/persistence/idb'
import { syncSidebarFromCache } from '@/lib/store/sidebar-sync'
import { useSidebarStore } from '@/lib/store/sidebar'
import type { RepoDTO, WorkspaceDTO } from '@/lib/types'

// Regression for the §14 add-repo bounce: after adding a repo the modal
// navigates to /ide/:p/:r/:w, but the route guard (which reads the sidebar
// store) treated the just-added workspace as unknown and bounced back to /
// because the sidebar tree had not been rebuilt yet. syncSidebarFromCache does
// an immediate, un-collapsed rebuild from the entity cache so the caller can
// guarantee the workspace is in the sidebar BEFORE it navigates.
function workspaceDTO(over: Partial<WorkspaceDTO> & { id: string; repoId: string }): WorkspaceDTO {
  return {
    projectId: 'p1',
    branch: 'develop',
    parentId: '',
    forkPointSha: '',
    status: 'new',
    working: false,
    lastError: '',
    added: 0,
    deleted: 0,
    mergeStrategy: 'merge',
    canMergeLocally: false,
    parentBranch: '',
    prUrl: '',
    prTitle: '',
    prTargetBranch: '',
    ...over,
  }
}

function repoDTO(over: Partial<RepoDTO> & { id: string }): RepoDTO {
  return {
    projectId: 'p1',
    name: 'crowbar',
    path: '/tmp/crowbar',
    defaultBranch: 'develop',
    avatarLabel: 'C',
    avatarColor: 'avatar-pink',
    avatarUrl: '',
    avatarEmoji: '',
    ...over,
  }
}

beforeEach(async () => {
  await wipeEntityCache()
  useSidebarStore.setState({ repos: [] })
})

describe('syncSidebarFromCache', () => {
  it('rebuilds the sidebar repo tree from the entity cache so a just-written workspace is present', async () => {
    await upsertEntity('crowbar_repos', repoDTO({ id: 'repo-9' }))
    await upsertEntity('crowbar_workspaces', workspaceDTO({ id: 'ws-1', repoId: 'repo-9' }))

    await syncSidebarFromCache()

    const repos = useSidebarStore.getState().repos
    const repo = repos.find((r) => r.id === 'repo-9')
    expect(repo).toBeDefined()
    expect(repo!.workspaces.some((w) => w.id === 'ws-1')).toBe(true)
  })
})
