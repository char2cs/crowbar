import { act, renderHook } from '@testing-library/react'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import type { NativeChromeIntent, NativeTabs } from '@/lib/native-chrome'
import { useHomeTreeStore, type HomeTree } from '@/lib/store/home-tree'
import {
  resetWindowPaneStoreForTests,
  windowPaneStore,
} from '@/features/panes/stores/window-pane-store'

const chrome = vi.hoisted(() => ({
  pushed: [] as NativeTabs[],
  listeners: new Set<(intent: NativeChromeIntent) => void>(),
}))
vi.mock('@/lib/native-chrome', () => ({
  setNativeTabs: (tabs: NativeTabs) => chrome.pushed.push(tabs),
  onNativeChromeIntent: (cb: (intent: NativeChromeIntent) => void) => {
    chrome.listeners.add(cb)
    return () => chrome.listeners.delete(cb)
  },
}))
vi.mock('@/lib/persistence/workspace-layout', () => ({
  saveWorkspaceLayout: vi.fn().mockResolvedValue(undefined),
}))

const { setBufferContent } = await import('@/features/editor/lib/buffer-save')
const { useNativeTabs } = await import('@/features/window/hooks/use-native-tabs')

const send = (intent: NativeChromeIntent) =>
  act(() => chrome.listeners.forEach((listener) => listener(intent)))
const last = () => chrome.pushed.at(-1)!
const pane = () => {
  const s = windowPaneStore.getState()
  return s.panes[s.activePaneId]
}
const mount = () => renderHook(() => useNativeTabs(pane().id, 'ws-1'))

beforeEach(() => {
  chrome.pushed.length = 0
  chrome.listeners.clear()
  resetWindowPaneStoreForTests()
  act(() =>
    windowPaneStore.getState().paneActions.openChat('c1', { projectId: 'p1', workspaceId: 'ws-1' }),
  )
})

describe('useNativeTabs', () => {
  it('offers a terminal and the branch review from +, and pushes an empty strip when it goes', () => {
    const { unmount } = mount()
    expect(last()).toEqual({
      tabs: [],
      newTabActions: [
        { id: 'terminal', title: 'New Terminal', symbol: 'apple.terminal' },
        { id: 'review', title: 'Review Branch', symbol: 'arrow.triangle.pull' },
      ],
    })
    unmount()
    expect(last()).toEqual({ tabs: [], newTabActions: [] })
  })

  // The desktop's gate: a home chat owns no branch, so + offers no review for it.
  it('offers the branch review only for a chat on a branch', () => {
    useHomeTreeStore.setState({
      trees: { p1: { chats: [{ id: 'c1' }], folders: [] } as unknown as HomeTree },
    })
    mount()
    expect(last().newTabActions.map((a) => a.id)).toEqual(['terminal'])
    useHomeTreeStore.setState({ trees: {} })
  })

  it('opens what + picks as the active tab, and marks each tab’s kind', () => {
    mount()
    send({ type: 'newTab', actionId: 'terminal' })
    send({ type: 'newTab', actionId: 'review' })
    expect(last().tabs.map((t) => [t.kind, t.active, t.closable])).toEqual([
      ['terminal', false, true],
      ['review', true, true],
    ])
  })

  it('switches to and closes the tab the strip names', () => {
    mount()
    send({ type: 'newTab', actionId: 'terminal' })
    send({ type: 'newTab', actionId: 'terminal' })
    const [first, second] = last().tabs
    send({ type: 'selectTab', id: first.id })
    expect(last().tabs.find((t) => t.active)?.id).toBe(first.id)
    send({ type: 'closeTab', id: second.id })
    expect(last().tabs.map((t) => t.id)).toEqual([first.id])
    expect(windowPaneStore.getState().buffers.map((b) => b.id)).toEqual([first.id])
  })

  it('says which file a file tab holds, and whether it has unsaved edits', () => {
    mount()
    act(() => {
      windowPaneStore.getState().bufferActions.openContent(
        {
          type: 'editor',
          path: '/r/src/pricing.ts',
          name: 'pricing.ts',
          content: 'saved',
          workspaceId: 'ws-1',
        },
        { paneId: pane().id },
      )
    })
    act(() => setBufferContent(pane().activeEditorTabId!, 'edited'))
    expect(last().tabs).toEqual([
      expect.objectContaining({
        title: 'pricing.ts',
        kind: 'file',
        path: '/r/src/pricing.ts',
        dirty: true,
      }),
    ])
  })
})
