import { beforeEach, describe, expect, it } from 'vitest'
import { allowedModes } from '@/components/sidebar/lib/sidebar-drop-policy'
import { NO_MODES, REORDER_MODES } from '@/components/tree-dnd/drop-core'
import { getInitialState, useSidebarStore } from '@/lib/store/sidebar'
import type { SidebarRow } from '@/components/sidebar/types/sidebar-row'

/**
 * Regression for "no reorder indicator between two repo header rows":
 * `use-sidebar-drag.ts`'s live per-frame hit test reconstructs its target
 * straight off DOM attributes (`tree-dnd/drop-dom.ts`'s `read()`), using only
 * the fields `SIDEBAR_DRAG_ROW_SPEC` declares — `kind`/`id`/`parentId` plus
 * `path`/`expanded`/`hasChildren`/`inRecents`. `repoIcon` is never one of
 * them, so a real drag's target NEVER carries it, unlike the full
 * `SidebarRow` object the sibling `sidebar-drop-policy.test.ts` file builds
 * by hand (which is why that suite never caught this: it always handed
 * `repoIcon` in). `sidebar-drop-policy.ts` used to trust `target.repoIcon`
 * for exactly this pairing, so the indicator always resolved to `NO_MODES`
 * live even though the same pairing "worked" in every hand-built test.
 */
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
    useSidebarStore.setState({
      ...getInitialState(),
      repos: [
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
      ],
    })
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
