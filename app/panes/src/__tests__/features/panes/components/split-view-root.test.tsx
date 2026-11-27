import { createElement, useEffect } from 'react'
import { act, render, screen } from '@testing-library/react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { windowPaneStore } from '@/features/panes/stores/window-pane-store'
import { resetWindowPaneStoreForTests } from '@/__tests__/__fixtures__/window-pane-store'
import { ROOT_PANE_ID } from '@/features/panes/constants/pane'
import { chatPaneIndex } from '@/features/panes/lib/view-selectors'

vi.mock('@/lib/persistence/workspace-layout', () => ({
  saveWorkspaceLayout: vi.fn().mockResolvedValue(undefined),
}))

const { mounts } = vi.hoisted(() => ({ mounts: { current: new Map<string, number>() } }))
vi.mock('@/features/panes/components/pane-container', () => ({
  PaneContainer: ({ pane, showing }: { pane: { id: string }; showing?: boolean }) => {
    useEffect(() => {
      mounts.current.set(pane.id, (mounts.current.get(pane.id) ?? 0) + 1)
    }, [pane.id])
    return createElement('div', {
      'data-testid': `pane-${pane.id}`,
      'data-showing': String(showing ?? true),
    })
  },
}))

vi.mock('@/features/window/stores/ui-state-store', () => ({
  useUIState: (sel: (s: { isBottomPaneVisible: boolean }) => unknown) =>
    sel({ isBottomPaneVisible: false }),
}))

import { SplitViewRoot } from '@/features/panes/components/split-view-root'

const mountsOf = (paneId: string) => mounts.current.get(paneId) ?? 0

beforeEach(() => {
  vi.useFakeTimers()
  mounts.current = new Map()
  resetWindowPaneStoreForTests()
})
afterEach(() => {
  vi.useRealTimers()
  resetWindowPaneStoreForTests()
})

async function mountParkedViews() {
  for (let slot = 0; slot < 5; slot++) {
    await act(async () => {
      await vi.runAllTimersAsync()
    })
  }
}

function openSecondView(): { pane: string; view: string } {
  const { paneActions } = windowPaneStore.getState()
  paneActions.openChat('chat-1')
  paneActions.openChat('chat-2')
  const pane = chatPaneIndex(windowPaneStore.getState().panes).get('chat-2')!
  return { pane, view: windowPaneStore.getState().panes[pane].viewId! }
}

describe('SplitViewRoot — only the active view occupies the content area', () => {
  it('renders every open view, and hides all but the showing one', async () => {
    const { pane: second, view: secondView } = openSecondView()
    await act(async () => {
      render(createElement(SplitViewRoot))
    })
    await mountParkedViews()

    const showing = screen.getByTestId(`pane-${second}`)
    const parked = screen.getByTestId(`pane-${ROOT_PANE_ID}`)
    expect(showing.getAttribute('data-showing')).toBe('true')
    expect(parked.getAttribute('data-showing')).toBe('false')

    const parkedRoot = document.querySelector(`[data-view-root="${ROOT_PANE_ID}"]`)!
    expect(parkedRoot.getAttribute('style')).toContain('display: none')
    expect(parkedRoot.hasAttribute('inert')).toBe(true)
    expect(document.querySelector(`[data-view-root="${secondView}"]`)!.hasAttribute('inert')).toBe(
      false,
    )
  })

  it('switching views does NOT remount either view — parking is not a teardown', async () => {
    const { pane: second, view: secondView } = openSecondView()
    await act(async () => {
      render(createElement(SplitViewRoot))
    })
    await mountParkedViews()
    expect(mountsOf(ROOT_PANE_ID)).toBe(1)
    expect(mountsOf(second)).toBe(1)

    await act(async () => {
      windowPaneStore.getState().paneActions.activateView(ROOT_PANE_ID)
    })
    expect(screen.getByTestId(`pane-${ROOT_PANE_ID}`).getAttribute('data-showing')).toBe('true')
    expect(screen.getByTestId(`pane-${second}`).getAttribute('data-showing')).toBe('false')

    await act(async () => {
      windowPaneStore.getState().paneActions.activateView(secondView)
    })

    expect(mountsOf(ROOT_PANE_ID)).toBe(1)
    expect(mountsOf(second)).toBe(1)
  })

  it('a view keeps its DOM node across a switch', async () => {
    openSecondView()
    await act(async () => {
      render(createElement(SplitViewRoot))
    })
    await mountParkedViews()
    const before = screen.getByTestId(`pane-${ROOT_PANE_ID}`)

    await act(async () => {
      windowPaneStore.getState().paneActions.activateView(ROOT_PANE_ID)
    })

    expect(screen.getByTestId(`pane-${ROOT_PANE_ID}`)).toBe(before)
  })

  it('a newly opened view is the only one showing', async () => {
    openSecondView()

    await act(async () => {
      render(createElement(SplitViewRoot))
    })
    await mountParkedViews()

    let third = ''
    await act(async () => {
      windowPaneStore.getState().paneActions.openChat('chat-3')
      const pane = chatPaneIndex(windowPaneStore.getState().panes).get('chat-3')!
      third = windowPaneStore.getState().panes[pane].viewId!
    })

    const showing = [...document.querySelectorAll('[data-view-root]')].filter(
      (e) => !e.hasAttribute('data-parked-view'),
    )
    expect(showing).toHaveLength(1)
    expect(showing[0].getAttribute('data-view-root')).toBe(third)
  })

  it('closing the showing view reveals the one behind it, without remounting it', async () => {
    const { pane: second, view: secondView } = openSecondView()
    await act(async () => {
      render(createElement(SplitViewRoot))
    })
    await mountParkedViews()
    expect(mountsOf(ROOT_PANE_ID)).toBe(1)

    await act(async () => {
      windowPaneStore.getState().paneActions.closeView(secondView)
    })

    expect(screen.queryByTestId(`pane-${second}`)).toBeNull()
    expect(screen.getByTestId(`pane-${ROOT_PANE_ID}`).getAttribute('data-showing')).toBe('true')
    expect(mountsOf(ROOT_PANE_ID)).toBe(1)
  })

  it('a chat landing in the stage promotes it without remounting the stage pane', async () => {
    await act(async () => {
      render(createElement(SplitViewRoot))
    })
    const before = screen.getByTestId(`pane-${ROOT_PANE_ID}`)

    await act(async () => {
      windowPaneStore.getState().paneActions.openChat('chat-1')
    })

    expect(windowPaneStore.getState().activeViewId).toBe(ROOT_PANE_ID)
    expect(screen.getByTestId(`pane-${ROOT_PANE_ID}`)).toBe(before)
    expect(mountsOf(ROOT_PANE_ID)).toBe(1)
  })
})

describe('SplitViewRoot — parked views mount after the showing one', () => {
  it('first commit renders only the showing view; parked ones follow in idle slots', async () => {
    const { pane: second } = openSecondView()
    await act(async () => {
      render(createElement(SplitViewRoot))
    })

    expect(screen.getByTestId(`pane-${second}`)).toBeTruthy()
    expect(screen.queryByTestId(`pane-${ROOT_PANE_ID}`)).toBeNull()

    await mountParkedViews()
    expect(screen.getByTestId(`pane-${ROOT_PANE_ID}`).getAttribute('data-showing')).toBe('false')
  })

  it('activating a view that is not mounted yet mounts it at once', async () => {
    openSecondView()
    await act(async () => {
      render(createElement(SplitViewRoot))
    })

    await act(async () => {
      windowPaneStore.getState().paneActions.activateView(ROOT_PANE_ID)
    })

    expect(screen.getByTestId(`pane-${ROOT_PANE_ID}`).getAttribute('data-showing')).toBe('true')
  })

  it('a view that was showing stays mounted once it parks', async () => {
    const { pane: second } = openSecondView()
    await act(async () => {
      render(createElement(SplitViewRoot))
    })
    await act(async () => {
      windowPaneStore.getState().paneActions.activateView(ROOT_PANE_ID)
    })
    await act(async () => {
      windowPaneStore
        .getState()
        .paneActions.activateView(windowPaneStore.getState().panes[second].viewId!)
    })

    expect(screen.getByTestId(`pane-${ROOT_PANE_ID}`).getAttribute('data-showing')).toBe('false')
    expect(mountsOf(ROOT_PANE_ID)).toBe(1)
  })

  it('mounts parked views most recently used first', async () => {
    const { paneActions } = windowPaneStore.getState()
    for (const chat of ['chat-1', 'chat-2', 'chat-3']) paneActions.openChat(chat)
    const paneOf = (chat: string) => chatPaneIndex(windowPaneStore.getState().panes).get(chat)!
    const viewOf = (chat: string) => windowPaneStore.getState().panes[paneOf(chat)].viewId!
    paneActions.activateView(viewOf('chat-1'))
    paneActions.activateView(viewOf('chat-3'))

    await act(async () => {
      render(createElement(SplitViewRoot))
    })
    await act(async () => {
      await vi.advanceTimersToNextTimerAsync()
    })

    expect(screen.queryByTestId(`pane-${paneOf('chat-1')}`)).not.toBeNull()
    expect(screen.queryByTestId(`pane-${paneOf('chat-2')}`)).toBeNull()
  })
})
