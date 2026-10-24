import { act, render, screen } from '@testing-library/react'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { PaneEditorViewContent } from '@/features/panes/components/pane-editor-view-content'
import { getEditorPortalEntry } from '@/features/panes/lib/editor-portal-registry'
import {
  resetWindowPaneStoreForTests,
  windowPaneStore,
} from '@/features/panes/stores/window-pane-store'
import type { PaneContent } from '@/features/panes/types/pane-content'
import { useSidebarFocusMode } from '@/components/layout/use-sidebar-focus-mode'
import { getInitialState, useSidebarStore } from '@/lib/store/sidebar'
import { publishFocusedWorkspaceContext } from '@/features/window/stores/focused-workspace-context-store'

vi.mock('@/features/panes/components/terminal-pane', () => ({
  TerminalPane: () => <textarea aria-label="terminal input" />,
}))

vi.mock('@/features/git/components/branch-review-pane', () => ({
  BranchReviewPane: () => <button type="button">review file</button>,
}))

const BASE = { isPinned: false, workspaceId: 'w1' }
const EDITOR: PaneContent = {
  ...BASE,
  id: 'ed',
  type: 'editor',
  path: '/a.ts',
  name: 'a.ts',
  content: '',
  savedContent: '',
  isDirty: false,
  isVirtual: false,
  tokens: [],
}
const TERMINAL: PaneContent = { ...BASE, id: 'term', type: 'terminal', name: 'zsh', sessionId: 's' }
const REVIEW: PaneContent = { ...BASE, id: 'rev', type: 'branchReview', name: 'Review', wsId: 'w1' }

function Host({ activeTab }: { activeTab: string }) {
  useSidebarFocusMode()
  return (
    <PaneEditorViewContent
      paneId="p1"
      editorTabIds={['ed', 'term', 'rev']}
      activeEditorTabId={activeTab}
      editorViewHidden={false}
      isActivePane
      showing
    />
  )
}

const mode = () => useSidebarStore.getState().activeTab

describe('PaneEditorViewContent sidebar surfaces', () => {
  beforeEach(() => {
    resetWindowPaneStoreForTests()
    windowPaneStore.setState((s) => {
      s.buffers.push(EDITOR, TERMINAL, REVIEW)
      return s
    })
    useSidebarStore.setState(getInitialState())
    publishFocusedWorkspaceContext({
      projectId: 'p1',
      workspaceId: 'w1',
      isProjectHome: false,
      repoId: 'r1',
      repoPath: '/repo',
      rootPath: '/repo',
    })
  })

  it('focus in the editor widget shows Files', () => {
    render(<Host activeTab="ed" />)
    const widget = document.createElement('textarea')
    getEditorPortalEntry('p1')!.node.appendChild(widget)
    act(() => widget.focus())
    expect(mode()).toBe('files')
  })

  it('focus in a review diff shows Git', async () => {
    render(<Host activeTab="rev" />)
    const diff = await screen.findByRole('button', { name: 'review file' })
    act(() => diff.focus())
    expect(mode()).toBe('git')
  })

  it('focus in a terminal shows Chats', () => {
    useSidebarStore.getState().setActiveTab('files')
    render(<Host activeTab="term" />)
    act(() => screen.getByLabelText('terminal input').focus())
    expect(mode()).toBe('chats')
  })
})
