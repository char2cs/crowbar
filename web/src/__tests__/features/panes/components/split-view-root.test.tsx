import { createElement, useEffect } from 'react'
import { act, render, screen } from '@testing-library/react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import {
  windowPaneStore,
  resetWindowPaneStoreForTests,
} from '@/features/panes/stores/window-pane-store'
import { ROOT_PANE_ID } from '@/features/panes/constants/pane'
import { chatPaneIndex } from '@/features/panes/lib/view-selectors'

vi.mock('@/lib/persistence/workspace-layout', () => ({
  saveWorkspaceLayout: vi.fn().mockResolvedValue(undefined),
}))

/**
 * A pane stood in by a marker that counts its own EFFECT-mounts — the same
 * instrument `pane-container.test.tsx` uses for the identical question, and
 * the only one that can tell "re-rendered" from "torn down and rebuilt". A
 * render counter would not: React re-renders a surviving component too.
 */
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

// Neither is relevant to which view renders, and both drag in real chrome.
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

/** Parked views mount one idle slot at a time after the showing view. */
async function mountParkedViews() {
  // Each slot's effect re-arms the next one only after React commits it.
  for (let slot = 0; slot < 5; slot++) {
    await act(async () => {
      await vi.runAllTimersAsync()
    })
  }
}

/** Two views, each one pane. The first is the promoted stage, so its view id
 *  is `ROOT_PANE_ID` too; returns the second's pane and view ids. */
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

    // The parked view's wrapper is what removes it from the layout — and it
    // is `inert`, so nothing off screen is focusable or clickable.
    const parkedRoot = document.querySelector(`[data-view-root="${ROOT_PANE_ID}"]`)!
    expect(parkedRoot.getAttribute('style')).toContain('display: none')
    expect(parkedRoot.hasAttribute('inert')).toBe(true)
    expect(document.querySelector(`[data-view-root="${secondView}"]`)!.hasAttribute('inert')).toBe(
      false,
    )
  })

  /**
   * THE REGRESSION. Rendering the showing view in one place and the parked
   * ones in another keeps every view "mounted" in the loosest sense and is
   * still wrong: switching moves a subtree between two parents, which React
   * can only do by unmounting it and mounting a new one. Measured live, that
   * destroyed a parked view's xterm — and because xterm re-initialisation is
   * gated on `isVisible`, it never came back.
   */
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

    // ...and back again, so neither direction of the swap is a teardown.
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

    // Identity, not just presence: a remount would hand back a different node
    // even though the query still finds one.
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
    // The revealed view was parked, not rebuilt — it is the same instance it
    // has been since it was opened.
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
    // chat-1 was used after chat-2; chat-3 is showing.
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
