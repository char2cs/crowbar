import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { useStore } from 'zustand'
import { ROOT_PANE_ID } from '@/features/panes/constants/pane'
import { PaneEditorViewContent } from '@/features/panes/components/pane-editor-view-content'
import { windowPaneStore } from '@/features/panes/stores/window-pane-store'
import { resetWindowPaneStoreForTests } from '@/__tests__/__fixtures__/window-pane-store'

vi.mock('@/features/panes/components/terminal-pane', () => ({
  TerminalPane: () => <textarea aria-label="terminal input" />,
}))

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
