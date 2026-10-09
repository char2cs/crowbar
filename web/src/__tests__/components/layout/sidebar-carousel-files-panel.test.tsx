import { act, render } from '@testing-library/react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

const tree = vi.hoisted(() => ({
  onRevealInFinder: undefined as undefined | ((p: string) => void),
}))
vi.mock('@/features/file-explorer/components/file-explorer-tree', () => ({
  FileExplorerTree: (props: { onRevealInFinder?: (path: string) => void }) => {
    tree.onRevealInFinder = props.onRevealInFinder
    return null
  },
}))

import { SidebarCarouselFilesPanel } from '@/components/layout/sidebar-carousel-files-panel'
import {
  EMPTY_FOCUSED_WORKSPACE_CONTEXT,
  useFocusedWorkspaceContextStore,
} from '@/features/window/stores/focused-workspace-context-store'
import { __resetWorkspaceScopesForTest, recordWorkspaceScope } from '@/lib/workspace-scope'
import {
  loadProjects,
  pairRemoteHost,
  projectOn,
  resetHostWorld,
} from '@/__tests__/__fixtures__/remote-host'

function focus(workspaceId: string, projectId: string) {
  recordWorkspaceScope({ projectId, repoId: 'r1', wsId: workspaceId })
  act(() =>
    useFocusedWorkspaceContextStore.setState({
      ...EMPTY_FOCUSED_WORKSPACE_CONTEXT,
      projectId,
      workspaceId,
      rootPath: '/work/root',
    }),
  )
}

// A remote workspace's paths mean nothing to this machine's Finder: the explorer must
// not offer to reveal them there.
describe('SidebarCarouselFilesPanel reveal in Finder', () => {
  beforeEach(() => {
    tree.onRevealInFinder = undefined
    resetHostWorld()
    __resetWorkspaceScopesForTest()
    pairRemoteHost('host-1')
    loadProjects(projectOn('p-local', 'local'), projectOn('p-remote', 'host-1'))
  })
  afterEach(() => {
    act(() => useFocusedWorkspaceContextStore.setState({ ...EMPTY_FOCUSED_WORKSPACE_CONTEXT }))
    resetHostWorld()
  })

  it('offers it for a workspace on the local host', () => {
    focus('ws-local', 'p-local')
    render(<SidebarCarouselFilesPanel />)

    expect(tree.onRevealInFinder).toBeTypeOf('function')
  })

  it('offers nothing for a workspace on a remote host', () => {
    focus('ws-remote', 'p-remote')
    render(<SidebarCarouselFilesPanel />)

    expect(tree.onRevealInFinder).toBeUndefined()
  })

  it('offers nothing while the workspace’s host is not known yet', () => {
    resetHostWorld()
    pairRemoteHost('host-1')
    focus('ws-local', 'p-local')
    render(<SidebarCarouselFilesPanel />)

    expect(tree.onRevealInFinder).toBeUndefined()
  })
})
