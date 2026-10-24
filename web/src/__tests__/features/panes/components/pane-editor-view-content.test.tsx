import { act, fireEvent, render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { useStore } from 'zustand'
import { ROOT_PANE_ID } from '@/features/panes/constants/pane'
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

// Rendered prose with nothing focusable, like the real read-only preview.
vi.mock('@/features/editor/markdown/markdown-preview', () => ({
  MarkdownPreview: () => <p>rendered markdown</p>,
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
const PREVIEW: PaneContent = {
  ...BASE,
  id: 'md',
  type: 'markdownPreview',
  name: 'README.md',
  content: '# hi',
  sourceFilePath: '/README.md',
}

function Host({ activeTab }: { activeTab: string }) {
  useSidebarFocusMode()
  return (
    <PaneEditorViewContent
      paneId="p1"
      wsId={null}
      editorTabIds={['ed', 'term', 'rev', 'md']}
      activeEditorTabId={activeTab}
      editorViewHidden={false}
      isActivePane
      showing
    />
  )
}

const mode = () => useSidebarStore.getState().activeTab

// Focus only moves the mode once the user has pressed something.
function renderHost(activeTab: string) {
  render(<Host activeTab={activeTab} />)
  fireEvent.pointerDown(document.body)
}

describe('PaneEditorViewContent sidebar surfaces', () => {
  beforeEach(() => {
    resetWindowPaneStoreForTests()
    windowPaneStore.setState((s) => {
      s.buffers.push(EDITOR, TERMINAL, REVIEW, PREVIEW)
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
    renderHost('ed')
    const widget = document.createElement('textarea')
    getEditorPortalEntry('p1')!.node.appendChild(widget)
    act(() => widget.focus())
    expect(mode()).toBe('files')
  })

  it('focus in a review diff shows Git', async () => {
    renderHost('rev')
    const diff = await screen.findByRole('button', { name: 'review file' })
    act(() => diff.focus())
    expect(mode()).toBe('git')
  })

  it('a click on a preview with nothing focusable under the pointer shows Files', async () => {
    render(<Host activeTab="md" />)
    await userEvent.click(await screen.findByText('rendered markdown'))
    expect(mode()).toBe('files')
  })

  it('focus in a terminal shows Chats', () => {
    useSidebarStore.getState().setActiveTab('files')
    renderHost('term')
    act(() => screen.getByLabelText('terminal input').focus())
    expect(mode()).toBe('chats')
  })
})

function LivePane() {
  const pane = useStore(windowPaneStore, (s) => s.panes[ROOT_PANE_ID])
  return (
    <PaneEditorViewContent
      paneId={ROOT_PANE_ID}
      wsId="w1"
      editorTabIds={pane.editorTabIds}
      activeEditorTabId={pane.activeEditorTabId}
      editorViewHidden={false}
      isActivePane
      showing
    />
  )
}

describe('PaneEditorViewContent New Tab page', () => {
  beforeEach(() => {
    resetWindowPaneStoreForTests()
    windowPaneStore.getState().paneActions.openChat('chat-1')
  })

  it('shows the new-tab actions, and a choice replaces the page', async () => {
    const page = windowPaneStore
      .getState()
      .bufferActions.openContent({ type: 'newTabPage', workspaceId: 'w1' })
    render(<LivePane />)

    await userEvent.click(screen.getByRole('button', { name: /new terminal/i }))

    const { panes, buffers } = windowPaneStore.getState()
    expect(panes[ROOT_PANE_ID].editorTabIds).toHaveLength(1)
    expect(buffers.map((b) => b.type)).toEqual(['terminal'])
    expect(buffers.some((b) => b.id === page)).toBe(false)
    expect(screen.getByLabelText('terminal input')).toBeInTheDocument()
  })
})
