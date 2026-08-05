/**
 * What a selection click costs to paint.
 *
 * Crowbar's whole case is being fast, and selection is the interaction most
 * likely to be pressed twenty times in a row. A cmd-click changes the answer to
 * "am I selected?" for exactly two rows, so exactly two rows may re-render — and
 * the way that is achieved is the narrow boolean selector every row subscribes
 * with, not a memo bolted on afterwards.
 *
 * Renders are counted through the leading glyph, which every workspace row
 * renders exactly once and which nothing else in the tree draws. Counting a
 * module rather than a prop callback because React may replay or discard render
 * work, and a callback passed IN would be part of what is replayed.
 */
import { describe, it, expect, beforeEach, vi } from 'vitest'
import { render, fireEvent } from '@testing-library/react'

const renders = vi.hoisted(() => ({ count: 0 }))

vi.mock('@tanstack/react-router', () => ({
  useNavigate: () => () => {},
  useRouterState: () => '',
  useRouter: () => ({ state: { location: { pathname: '/' } } }),
  useMatch: () => null,
}))

vi.mock('@/components/layout/workspace-branch-icon', () => ({
  WorkspaceBranchIcon: () => {
    renders.count += 1
    return null
  },
  WorkspaceAgentSpinner: () => null,
}))

import { idle, success } from '@/lib/loadable'
import { useWorkspaceListStore } from '@/lib/store/workspace-list'
import { useProjectDataStore } from '@/lib/store/projects'
import { useHomeWorkspaceStore } from '@/lib/store/home-workspace'
import { useSidebarStore, type Repo } from '@/lib/store/sidebar'
import { getInitialSelectionState, useSidebarSelectionStore } from '@/lib/store/sidebar-selection'
import { WorkspaceTree } from '@/components/layout/workspace-tree'
import type { Project } from '@/lib/types'

/** Eight sibling rows, so "two re-rendered" is distinguishable from "all did". */
const repo = (): Repo => ({
  id: 'r1',
  projectId: 'p1',
  name: 'crowbar',
  avatarLabel: 'C',
  avatarColor: 'bg-indigo-700',
  defaultWorkspaceId: 'w-default',
  workspaces: Array.from({ length: 8 }, (_, i) => ({
    id: `w${i}`,
    branch: `feature/${i}`,
    status: 'new' as const,
    age: '1d',
    order: i,
  })),
})

const project: Project = {
  id: 'p1',
  name: 'crowbar-project',
  path: '/p1',
  lastActivity: new Date(0),
}

const rowFor = (id: string) => document.querySelector<HTMLElement>(`[data-ws-drop="${id}"]`)!

beforeEach(() => {
  renders.count = 0
  useWorkspaceListStore.setState({ data: idle() })
  useHomeWorkspaceStore.setState({ workspace: null })
  useProjectDataStore.setState({ data: success([project]) })
  useSidebarSelectionStore.setState(getInitialSelectionState())
  useSidebarStore.setState({
    repos: [repo()],
    collapsedRepos: new Set<string>(),
    collapsedWorkspaces: new Set<string>(),
    collapsedProjects: new Set<string>(),
  })
})

describe('a selection click re-renders the rows it changed, and no others', () => {
  it('costs ONE row render to cmd-click one row out of eight', () => {
    render(<WorkspaceTree />)
    renders.count = 0

    fireEvent.click(rowFor('w3'), { metaKey: true })

    expect(renders.count).toBe(1)
  })

  it('costs TWO to move the selection from one row to another', () => {
    render(<WorkspaceTree />)
    fireEvent.click(rowFor('w3'), { metaKey: true })
    renders.count = 0

    fireEvent.click(rowFor('w3'), { metaKey: true })
    fireEvent.click(rowFor('w5'), { metaKey: true })

    expect(renders.count).toBe(2)
  })

  it('costs one render per lit row when a plain click clears them', () => {
    render(<WorkspaceTree />)
    fireEvent.click(rowFor('w1'), { metaKey: true })
    fireEvent.click(rowFor('w2'), { metaKey: true })
    fireEvent.click(rowFor('w3'), { metaKey: true })
    renders.count = 0

    // w7 is not lit, so it is not among the rows whose answer changes; the
    // three that were lit are.
    fireEvent.click(rowFor('w7'))

    expect(renders.count).toBe(3)
  })

  it('costs nothing at all to plain-click with nothing selected', () => {
    render(<WorkspaceTree />)
    renders.count = 0

    fireEvent.click(rowFor('w7'))

    expect(renders.count).toBe(0)
  })
})

describe('disclosure controls do not repaint rows whose data did not change', () => {
  it('collapses a repo without rendering any workspace row again', () => {
    render(<WorkspaceTree />)
    renders.count = 0

    useSidebarStore.getState().toggleRepo('r1')

    expect(renders.count).toBe(0)
  })
})
