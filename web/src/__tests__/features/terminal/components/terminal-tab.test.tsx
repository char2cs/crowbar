import { createElement } from 'react'
import { act, fireEvent, render, screen } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'
import { terminalKill } from '@/lib/crowbar-bridge'
import { useTerminalStore } from '@/features/terminal/stores/terminal-store'
import { BOTTOM_PANE_ID, ROOT_PANE_ID } from '@/features/panes/constants/pane'
import { WorkspaceStoreContext } from '@/features/workspace/stores/workspace-context'
import { createWorkspaceStore } from '@/features/workspace/stores/workspace-store'
import {
  windowPaneStore,
  resetWindowPaneStoreForTests,
} from '@/features/panes/stores/window-pane-store'

// jsdom can't run xterm/WebGL — stand in for it, same as agent-chat-pane's
// own suite. The stand-in keeps its props so a test can fire the exit frame.
const xterm = vi.hoisted(() => ({ props: {} as { onTerminalExit?: (id: string) => void } }))
vi.mock('@/features/terminal/components/terminal', () => ({
  XtermTerminal: (props: typeof xterm.props) => {
    xterm.props = props
    return createElement('div', { 'data-testid': 'xterm' })
  },
}))
vi.mock('@/lib/crowbar-bridge', () => ({ terminalKill: vi.fn(async () => {}) }))

import { TerminalTab } from '@/features/terminal/components/terminal-tab'

function seedTerminalBuffer(paneId: string, bufferId: string) {
  resetWindowPaneStoreForTests()
  windowPaneStore.setState((s) => {
    s.buffers = [
      {
        id: bufferId,
        type: 'terminal',
        name: 'Terminal',
        sessionId: 'pty-1',
        workspaceId: 'w1',
      },
    ]
    s.panes[paneId] = { ...s.panes[paneId], editorTabIds: [bufferId], activeEditorTabId: bufferId }
    return s
  })
}

function renderTab(paneId: string | undefined, bufferId: string) {
  const store = createWorkspaceStore('w1')
  return render(
    createElement(
      WorkspaceStoreContext.Provider,
      { value: store },
      createElement(TerminalTab, { sessionId: 'pty-1', bufferId, paneId, workspaceId: 'w1' }),
    ),
  )
}

// I8 (Task 26 fix round 1): handleActivate called paneActions.addBufferToPane/
// .activatePaneBuffer, neither of which has existed on PaneActions since Task
// 1's editorTabIds rename — every click into a pane terminal threw.
describe('TerminalTab — activating a pane terminal (I8)', () => {
  it('activates the tab and the pane it belongs to, without throwing, when paneId is known', () => {
    seedTerminalBuffer(BOTTOM_PANE_ID, 'buf-term-1')
    windowPaneStore.getState().paneActions.setActivePane(ROOT_PANE_ID)

    act(() => {
      renderTab(BOTTOM_PANE_ID, 'buf-term-1')
    })

    expect(() => {
      act(() => {
        fireEvent.mouseDown(screen.getByTestId('xterm').parentElement!)
      })
    }).not.toThrow()

    expect(windowPaneStore.getState().activePaneId).toBe(BOTTOM_PANE_ID)
    expect(windowPaneStore.getState().panes[BOTTOM_PANE_ID]?.activeEditorTabId).toBe('buf-term-1')
  })

  it('resolves the holding pane itself, without throwing, when paneId is not passed', () => {
    seedTerminalBuffer(BOTTOM_PANE_ID, 'buf-term-2')
    windowPaneStore.getState().paneActions.setActivePane(ROOT_PANE_ID)

    act(() => {
      renderTab(undefined, 'buf-term-2')
    })

    expect(() => {
      act(() => {
        fireEvent.mouseDown(screen.getByTestId('xterm').parentElement!)
      })
    }).not.toThrow()

    expect(windowPaneStore.getState().activePaneId).toBe(BOTTOM_PANE_ID)
  })
})

describe('TerminalTab — the shell exits', () => {
  it('closes the tab without asking the daemon to kill the PTY that already exited', async () => {
    seedTerminalBuffer(ROOT_PANE_ID, 'buf-term-3')
    useTerminalStore.setState({ sessions: new Map() })
    useTerminalStore.getState().updateSession('pty-1', { connectionId: 'daemon-pty-9' })
    act(() => {
      renderTab(ROOT_PANE_ID, 'buf-term-3')
    })

    act(() => {
      xterm.props.onTerminalExit?.('pty-1')
    })
    await vi.dynamicImportSettled()

    expect(windowPaneStore.getState().buffers.some((b) => b.id === 'buf-term-3')).toBe(false)
    expect(terminalKill).not.toHaveBeenCalled()
  })
})
