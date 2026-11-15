import { act, render } from '@testing-library/react'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { ROOT_PANE_ID } from '@/features/panes/constants/pane'
import { makePane } from '@/features/panes/lib/view-state'
import { windowPaneStore } from '@/features/panes/stores/window-pane-store'
import { bufferSession } from '@/entries/buffer-session'

vi.mock('@/features/agent/hooks/use-chat-entity', () => ({
  useChatField: (_chatId: string, pick: (chat: unknown) => unknown) => pick({ workspaceId: 'ws1' }),
}))
vi.mock('@/features/panes/components/pane-editor-view-content', () => ({
  PaneEditorViewContent: () => null,
}))
vi.mock('@/features/panes/components/editor-host-registry', () => ({
  EditorHostRegistry: () => null,
}))

const { BufferPaneHost } = await import('@/entries/buffer-pane-host')

afterEach(() => {
  bufferSession.setState({ chatId: null, saved: null, active: false })
  windowPaneStore.setState({ buffers: [] })
})

describe('BufferPaneHost', () => {
  it('opens the saved buffer in the chat workspace and titles the pane with the tab', async () => {
    const setTitle = vi.fn()
    const open = vi.fn((_ws: string, _saved: unknown) => {
      windowPaneStore.setState((s) => ({
        buffers: [...s.buffers, { id: 'b1', type: 'newTabPage', name: 'a.ts', workspaceId: 'ws1' }],
        panes: {
          ...s.panes,
          [ROOT_PANE_ID]: makePane(ROOT_PANE_ID, null, { editorTabIds: ['b1'] }),
        },
      }))
      return Promise.resolve('b1')
    })
    bufferSession.setState({ chatId: 'c1', saved: { path: 'a.ts' }, active: false })
    await act(async () => {
      render(<BufferPaneHost client={{ setTitle }} open={open} />)
    })
    expect(open).toHaveBeenCalledWith('ws1', { path: 'a.ts' })
    expect(setTitle).toHaveBeenCalledWith('a.ts')
  })
})
