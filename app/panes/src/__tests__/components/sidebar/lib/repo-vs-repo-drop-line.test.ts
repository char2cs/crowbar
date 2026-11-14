import { beforeEach, describe, expect, it } from 'vitest'
import { allowedModes } from '@/components/sidebar/lib/sidebar-drop-policy'
import { NO_MODES, REORDER_MODES } from '@/components/tree-dnd/drop-core'
import type { SidebarRow } from '@/components/sidebar/types/sidebar-row'
import { seedRepos } from '@/__tests__/__fixtures__/sidebar-entities'

// A live drag's target is rebuilt from the DOM attributes SIDEBAR_DRAG_ROW_SPEC declares, which
// never include `repoIcon` — so this builds targets that way, unlike the hand-built rows in
// sidebar-drop-policy.test.ts.
function liveDomTarget(id: string, parentId = ''): SidebarRow {
  return {
    kind: 'branch',
    id,
    parentId,
    expanded: false,
    hasChildren: false,
    inRecents: false,
  } as unknown as SidebarRow
}

function repoHeaderSubject(id: string, projectId: string, repoId: string): SidebarRow {
  return {
    id,
    kind: 'branch',
    parentId: null,
    order: 0,
    label: repoId,
    ownsWorktree: true,
    workspaceId: id,
    working: false,
    hasView: false,
    repoIcon: {
      repoId,
      projectId,
      name: repoId,
      avatarLabel: 'A',
      avatarColor: 'bg-indigo-700',
    },
  }
}

describe('repo-vs-repo reorder indicator, against a live DOM-reconstructed target', () => {
  beforeEach(() => {
    seedRepos([
      {
        id: 'repo-1',
        projectId: 'proj-1',
        name: 'repo-1',
        avatarLabel: 'A',
        avatarColor: 'bg-indigo-700',
        defaultWorkspaceId: 'home-1',
        workspaces: [],
      },
      {
        id: 'repo-2',
        projectId: 'proj-1',
        name: 'repo-2',
        avatarLabel: 'B',
        avatarColor: 'bg-indigo-700',
        defaultWorkspaceId: 'home-2',
        workspaces: [],
      },
      {
        id: 'repo-3',
        projectId: 'proj-2',
        name: 'repo-3',
        avatarLabel: 'C',
        avatarColor: 'bg-indigo-700',
        defaultWorkspaceId: 'home-3',
        workspaces: [],
      },
    ])
  })

  it('offers the reorder modes between two repo headers in the same project', () => {
    const subject = repoHeaderSubject('home-1', 'proj-1', 'repo-1')
    const target = liveDomTarget('home-2')
    expect(allowedModes([subject], target)).toEqual(REORDER_MODES)
  })

  it('still refuses between two repo headers in different projects', () => {
    const subject = repoHeaderSubject('home-1', 'proj-1', 'repo-1')
    const target = liveDomTarget('home-3')
    expect(allowedModes([subject], target)).toEqual(NO_MODES)
  })
})
