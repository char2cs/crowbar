import { describe, it, expect, vi, beforeEach } from 'vitest'
import { renderHook } from '@testing-library/react'

// Cmd+W must close the ACTIVE tab — not the app. Previously NO close-tab command
// existed, so on desktop Cmd+W fell through to the macOS default menu's
// Window > Close and quit Crowbar. This locks the registry binding -> the
// use-pane-keyboard dispatch that closes the active buffer instead.
vi.mock('@/utils/platform', async () => {
  const actual = await vi.importActual<typeof import('@/utils/platform')>('@/utils/platform')
  return { ...actual, IS_MAC: true }
})

const closeBuffer = vi.fn()
const reopenLastClosedBuffer = vi.fn()
const setPendingClose = vi.fn()
const removeBufferFromPane = vi.fn()
const navigateToPane = vi.fn()

const fakeState = {
  activePaneId: 'pane-1',
  panes: { 'pane-1': { activeBufferId: 'buf-1' as string | null } },
  buffers: [{ id: 'buf-1', type: 'editor', isDirty: false }] as Array<{
    id: string
    type: string
    isDirty: boolean
  }>,
  bufferActions: { closeBuffer, reopenLastClosedBuffer, setPendingClose },
  paneActions: { navigateToPane, removeBufferFromPane },
}
const fakeStore = { getState: () => fakeState }

vi.mock('@/features/workspace/stores/workspace-context', () => ({
  useWorkspaceStore: () => fakeStore,
}))

vi.mock('@/features/keymaps/hooks/use-effective-keymap', () => ({
  useEffectiveChordMap: () => ({
    'tabs.closeActive': 'mod+w',
    'tabs.reopenClosed': 'mod+shift+t',
    'panes.splitRight': 'mod+\\',
  }),
}))

vi.mock('@/features/panes/utils/pane-command-actions', () => ({
  splitActiveEditorGroup: vi.fn(),
}))

import { usePaneKeyboard } from '@/features/panes/hooks/use-pane-keyboard'

beforeEach(() => {
  vi.clearAllMocks()
  fakeState.panes = { 'pane-1': { activeBufferId: 'buf-1' } }
  fakeState.buffers = [{ id: 'buf-1', type: 'editor', isDirty: false }]
})

function pressCmdW() {
  window.dispatchEvent(
    new KeyboardEvent('keydown', { key: 'w', metaKey: true, bubbles: true, cancelable: true }),
  )
}

describe('usePaneKeyboard — Cmd+W closes the active tab', () => {
  it('removes the active buffer from its pane (so a neighbor activates) AND closes it', () => {
    renderHook(() => usePaneKeyboard())
    pressCmdW()
    // removeBufferFromPane is what activates the adjacent tab — without it the
    // pane is left with a dangling activeBufferId and falls to the empty state.
    expect(removeBufferFromPane).toHaveBeenCalledWith('pane-1', 'buf-1')
    expect(closeBuffer).toHaveBeenCalledWith('buf-1')
  })

  it('prompts (pendingClose) instead of closing a DIRTY editor buffer', () => {
    fakeState.buffers = [{ id: 'buf-1', type: 'editor', isDirty: true }]
    renderHook(() => usePaneKeyboard())
    pressCmdW()
    expect(setPendingClose).toHaveBeenCalledWith({ type: 'single', bufferId: 'buf-1' })
    expect(closeBuffer).not.toHaveBeenCalled()
    expect(removeBufferFromPane).not.toHaveBeenCalled()
  })

  it('does nothing on mod+w when there is no active buffer', () => {
    fakeState.panes = { 'pane-1': { activeBufferId: null } }
    renderHook(() => usePaneKeyboard())
    pressCmdW()
    expect(closeBuffer).not.toHaveBeenCalled()
    expect(removeBufferFromPane).not.toHaveBeenCalled()
  })
})
