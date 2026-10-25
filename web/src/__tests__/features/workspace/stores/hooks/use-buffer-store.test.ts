import { describe, it, expect, beforeEach, vi } from 'vitest'
import { act, renderHook } from '@testing-library/react'
import {
  resetWindowPaneStoreForTests,
  windowPaneStore,
} from '@/features/panes/stores/window-pane-store'
import { BOTTOM_PANE_ID } from '@/features/panes/constants/pane'
import { useTerminalBufferCount } from '@/features/workspace/stores/hooks/use-buffer-store'

vi.mock('@/features/terminal/lib/kill-terminal-session', () => ({
  killTerminalSession: vi.fn(async () => {}),
}))
vi.mock('@/features/terminal/lib/terminal-reconnect-map', () => ({
  clearReconnect: vi.fn(),
  saveReconnect: vi.fn(),
  loadReconnect: vi.fn(() => null),
}))
vi.mock('@/lib/persistence/workspace-layout', () => ({
  saveWorkspaceLayout: vi.fn().mockResolvedValue(undefined),
}))

const openTerminal = (sessionId: string) =>
  windowPaneStore
    .getState()
    .bufferActions.openContent(
      { type: 'terminal', sessionId, name: sessionId },
      { paneId: BOTTOM_PANE_ID },
    )

beforeEach(() => resetWindowPaneStoreForTests())

describe('useTerminalBufferCount', () => {
  it('counts the open terminal tabs and nothing else', () => {
    const { result } = renderHook(() => useTerminalBufferCount())
    expect(result.current).toBe(0)

    let first = ''
    act(() => {
      first = openTerminal('sess-1')
      openTerminal('sess-2')
      windowPaneStore.getState().bufferActions.openContent({
        type: 'editor',
        path: '/a.ts',
        name: 'a.ts',
        content: '',
      })
    })
    expect(result.current).toBe(2)

    act(() => {
      windowPaneStore.getState().paneActions.removeEditorTabFromPane(BOTTOM_PANE_ID, first)
      windowPaneStore.getState().bufferActions.closeBuffer(first)
    })
    expect(result.current).toBe(1)
  })
})
