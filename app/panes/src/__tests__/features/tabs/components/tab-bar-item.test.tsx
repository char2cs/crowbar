import { act, fireEvent, render, screen } from '@testing-library/react'
import { beforeEach, describe, it, expect, vi } from 'vitest'
import TabBarItem from '@/features/tabs/components/tab-bar-item'
import type { BranchReviewContent, EditorContent } from '@/features/panes/types/pane-content'
import { useSidebarFocusMode } from '@/components/layout/use-sidebar-focus-mode'
import { getDefaultSettingsSnapshot } from '@/features/settings/config/default-settings'
import { useSettingsStore } from '@/features/settings/store'
import { treeUiStore } from '@/features/workspace/tree/tree-ui-store'
import { publishFocusedWorkspaceContext } from '@/features/window/stores/focused-workspace-context-store'
import { resetEntities, seedRepos } from '@/__tests__/__fixtures__/sidebar-entities'

vi.mock('@/features/file-explorer/components/file-explorer-icon', () => ({
  FileExplorerIcon: () => <span data-testid="file-icon" />,
}))

const editorBuffer: EditorContent = {
  id: 'buf-1',
  type: 'editor',
  authority: 'workspace',
  path: '/project/bar.ts',
  name: 'bar.ts',
  isDirty: false,
  isVirtual: false,
  isPinned: false,
  isPreview: false,
  workspaceId: 'w1',
}

const shared = {
  displayName: 'bar.ts',
  index: 0,
  isDraggedTab: false,
  onDoubleClick: () => {},
  onContextMenu: () => {},
  onKeyDown: () => {},
  handleTabClose: () => {},
  handleTabPin: () => {},
}

describe('TabBarItem', () => {
  // Spec §7.1: there is no "Editor"/New Tab placeholder tab any more — the
  // sole-tab-in-a-pane invariant (isUncloseable) now applies to any real
  // editor-tab content, exercised here with a plain editor buffer.
  it('renders an uncloseable tab with its label and no close button', () => {
    const buffer: EditorContent = { ...editorBuffer, isUncloseable: true }
    render(<TabBarItem buffer={buffer} isActive {...shared} />)
    expect(screen.getByText('bar.ts')).toBeInTheDocument()
    expect(screen.queryByLabelText(/close/i)).not.toBeInTheDocument()
  })

  it('an inactive dirty tab shows the unsaved dot', () => {
    const buffer: EditorContent = { ...editorBuffer, isDirty: true }
    render(<TabBarItem buffer={buffer} isActive={false} {...shared} />)
    expect(screen.getByLabelText('Unsaved changes')).toBeInTheDocument()
  })
})

// A locked branch's file says so on its tab before the user types, not only when a save fails.
describe('TabBarItem on a locked workspace', () => {
  it('marks the tab read-only', () => {
    seedRepos([
      {
        id: 'r1',
        projectId: 'p1',
        name: 'repo',
        avatarLabel: 'R',
        avatarColor: 'r',
        workspaces: [],
        defaultWorkspaceId: 'w1',
        defaultWorkspaceStatus: 'locked',
      },
    ])
    render(<TabBarItem buffer={editorBuffer} isActive {...shared} />)

    expect(screen.getByRole('tab', { name: 'bar.ts (read-only)' })).toBeInTheDocument()
    resetEntities()
  })

  it('leaves an unlocked workspace’s tab alone', () => {
    render(<TabBarItem buffer={editorBuffer} isActive {...shared} />)

    expect(screen.getByRole('tab', { name: 'bar.ts' })).toBeInTheDocument()
  })
})

describe('TabBarItem sidebar surface', () => {
  // The mode follows focus only with the opt-in setting on.
  beforeEach(() => {
    useSettingsStore.setState({
      settings: { ...getDefaultSettingsSnapshot(), sidebarFollowsFocus: true },
    })
  })
  function FocusMode() {
    useSidebarFocusMode()
    return null
  }
  const reviewBuffer: BranchReviewContent = {
    id: 'review-1',
    type: 'branchReview',
    name: 'Branch Review',
    wsId: 'w1',
    isPinned: false,
    workspaceId: 'w1',
  }

  beforeEach(() => {
    treeUiStore.setState({ activeTab: 'chats' })
    publishFocusedWorkspaceContext({
      projectId: 'p1',
      workspaceId: 'w1',
      isProjectHome: false,
      repoId: 'r1',
      repoPath: '/repo',
      rootPath: '/repo',
    })
  })

  it('focusing an editor tab shows Files, and a review tab shows Git', () => {
    render(
      <>
        <FocusMode />
        <TabBarItem buffer={editorBuffer} isActive {...shared} />
        <TabBarItem buffer={reviewBuffer} isActive {...shared} displayName="Branch Review" />
      </>,
    )
    const [editorTab, reviewTab] = screen.getAllByRole('tab')
    // Focus only moves the mode once the user has pressed something.
    fireEvent.pointerDown(document.body)
    act(() => editorTab.focus())
    expect(treeUiStore.getState().activeTab).toBe('files')
    act(() => reviewTab.focus())
    expect(treeUiStore.getState().activeTab).toBe('git')
  })

  it('exposes the full title of a long tab to assistive tech and the hover tooltip', async () => {
    const title = 'Crowbar - Bring your own agent. Keep your own context. Always.'
    const browserBuffer = {
      id: 'b-1',
      type: 'browser',
      url: 'https://crowbar.test',
      name: title,
      path: 'browser://b-1',
      isPinned: false,
      isPreview: false,
      workspaceId: 'w1',
    } as const
    render(<TabBarItem buffer={browserBuffer} isActive {...shared} displayName={title} />)

    const tab = screen.getByRole('tab', { name: title })
    expect(tab.textContent).toBe(title)
    expect(screen.getAllByRole('button')).toHaveLength(1)

    fireEvent.pointerEnter(tab)
    fireEvent.mouseEnter(tab)
    fireEvent.focus(tab)
    expect(
      await screen.findByText(title, { selector: '[data-slot="tooltip-content"] *' }),
    ).toBeTruthy()
  })
})
