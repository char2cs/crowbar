import { createElement } from 'react'
import { act, render, screen } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'
import {
  WorkspaceStoreContext,
  useWorkspaceStoreContext,
} from '@/features/workspace/stores/workspace-context'
import { createWorkspaceStore } from '@/features/workspace/stores/workspace-store'
import { ROOT_PANE_ID } from '@/features/panes/constants/pane'

// A classic controlled Suspense "resource": renders nothing until resolved,
// then THROWS the pending promise (which React's nearest Suspense boundary
// catches) rather than resolving asynchronously on its own. This lets the test
// hold the lazy CommitDiffPane in a suspended state for as long as it wants and
// resolve it on command — no timers, no sleeps.
const { diffResource } = vi.hoisted(() => {
  function createDeferredResource<T>() {
    let status: 'pending' | 'resolved' = 'pending'
    let result: T
    let resolveFn!: (v: T) => void
    const promise = new Promise<T>((res) => {
      resolveFn = res
    })
    promise.then((v) => {
      status = 'resolved'
      result = v
    })
    return {
      read(): T {
        if (status === 'pending') throw promise
        return result
      },
      resolve: (v: T) => resolveFn(v),
    }
  }
  return { diffResource: createDeferredResource<true>() }
})

// pane-container.tsx lazy-loads `./commit-diff-pane` (relative import). Vitest/Vite
// mocks key off the resolved absolute module id, so mocking the SAME file via
// its '@/...' alias here intercepts it — the pattern already relied on by
// branch-section.test.tsx and agent-chat-pane.test.tsx for sibling relative
// imports. The mocked module resolves immediately; suspension is instead
// driven by `diffResource.read()` inside the component body below, so the
// test controls exactly when the lazy pane stops suspending.
vi.mock('@/features/panes/components/commit-diff-pane', () => ({
  CommitDiffPane: () => {
    diffResource.read()
    return createElement('div', { 'data-testid': 'diff-pane-content' }, 'Diff Loaded')
  },
}))

// TerminalPane is a heavy PTY-attached component (xterm, WebSocket attach).
// This test is about DOM-node identity across a sibling suspension, not
// terminal behavior — stub it to a passive marker element, mirroring how
// agent-chat-pane.test.tsx stubs XtermTerminal for the same reason.
vi.mock('@/features/panes/components/terminal-pane', () => ({
  TerminalPane: ({ sessionId }: { sessionId?: string }) =>
    createElement('div', { 'data-testid': 'terminal-marker', 'data-session-id': sessionId ?? '' }),
}))

// TabBar drags in SidebarProvider/dnd-kit machinery that has nothing to do
// with the question this test asks (does a sibling lazy suspension unmount
// the always-mounted terminal pane) — stub it out entirely.
vi.mock('@/features/tabs/components/tab-bar', () => ({
  default: () => null,
}))

import { PaneContainer } from '@/features/panes/components/pane-container'

function PaneHost() {
  const panes = useWorkspaceStoreContext((s) => s.panes)
  const pane = panes[ROOT_PANE_ID]
  if (!pane) return null
  return createElement(PaneContainer, { pane })
}

// bufferActions.openContent goes through buffer-slice's own (currently
// unmigrated, pre-existing-broken) addBufferToPane call — seed the buffers
// and editor tabs directly instead, the same way buffer-slice.test.ts and
// pane-slice.test.ts do, so this test exercises PaneContainer's OWN keep-alive
// wiring rather than buffer-slice's unrelated breakage.
function seedStore() {
  const store = createWorkspaceStore('w1')
  const terminalId = 'terminal-1'
  const diffId = 'diff-1'
  store.setState((state) => {
    state.buffers.push({
      id: terminalId,
      type: 'terminal',
      sessionId: 'pty-1',
      path: 'terminal://pty-1',
      name: 'Terminal 1',
      isPinned: false,
      isPreview: false,
    })
    state.buffers.push({
      id: diffId,
      type: 'commitDiff',
      wsId: 'w1',
      sha: 'abc1234',
      name: 'Commit abc1234',
      path: 'commit-diff://w1/abc1234',
      isPinned: false,
      isPreview: false,
    })
    return state
  })
  store.getState().paneActions.addEditorTabToPane(ROOT_PANE_ID, {
    id: terminalId,
    type: 'terminal',
    name: 'Terminal 1',
  })
  // Added AFTER the terminal, so addEditorTabToPane's own always-activates
  // behavior leaves this one — the lazy pane — as the pane's active tab.
  store.getState().paneActions.addEditorTabToPane(ROOT_PANE_ID, {
    id: diffId,
    type: 'commitDiff',
    name: 'Commit abc1234',
  })
  return { store, terminalId, diffId }
}

describe('PaneContainer Suspense boundary', () => {
  it('keeps the always-mounted terminal pane mounted while a sibling lazy pane suspends, and after it resolves', async () => {
    const { store } = seedStore()

    await act(async () => {
      render(
        createElement(WorkspaceStoreContext.Provider, { value: store }, createElement(PaneHost)),
      )
    })

    // The diff buffer is active and its lazy chunk is suspended (diffResource
    // has not resolved). Before the fix, the terminal map lived INSIDE the
    // same <Suspense> as the lazy switch, so this suspension would hide the
    // fallback (null) for the whole boundary — including the statically
    // imported, always-mounted TerminalPane sibling. Getting it here at all
    // (not queryBy) is the assertion that catches the regression.
    const terminalBefore = screen.getByTestId('terminal-marker')
    expect(terminalBefore).toHaveAttribute('data-session-id', 'pty-1')
    expect(screen.queryByTestId('diff-pane-content')).not.toBeInTheDocument()

    // Resolve the lazy pane's chunk.
    await act(async () => {
      diffResource.resolve(true)
    })

    expect(await screen.findByTestId('diff-pane-content')).toHaveTextContent('Diff Loaded')
    // Same DOM node — the terminal was never unmounted/remounted by the
    // sibling's suspend-then-resolve cycle.
    expect(screen.getByTestId('terminal-marker')).toBe(terminalBefore)
  })
})
