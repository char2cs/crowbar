import { createElement } from 'react'
import { act, render } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'
import { WorkspaceStoreContext } from '@/features/workspace/stores/workspace-context'
import { createWorkspaceStore } from '@/features/workspace/stores/workspace-store'

// WorkspaceHost keeps every retained WorkspaceView mounted at once (keep-alive),
// each wrapped in its OWN ambient WorkspaceStoreContext. TerminalTab used to read
// its workspace off that ambient context instead of its OWN buffer's
// TerminalContent.workspaceId — so a shell tab belonging to workspace A, rendered
// inside a HIDDEN copy of workspace B (kept alive underneath the active workspace),
// resolved its connection against B. On a genuine transport drop that spawns a
// fresh PTY in B's worktree and clobbers A's session record — the Pattern 2 shape
// from agent-chat-pane.tsx's `known` gate, applied to plain shell tabs. This test
// asserts TerminalTab threads its OWN prop through, ignoring the ambient context.
const { receivedProps } = vi.hoisted(() => ({
  receivedProps: [] as Array<{ workspaceId?: string; sessionId?: string }>,
}))

vi.mock('@/features/terminal/components/terminal', () => ({
  XtermTerminal: (props: { workspaceId?: string; sessionId?: string }) => {
    receivedProps.push({ workspaceId: props.workspaceId, sessionId: props.sessionId })
    return createElement('div', { 'data-testid': 'xterm' })
  },
}))

import { TerminalTab } from '@/features/terminal/components/terminal-tab'

describe('TerminalTab — resolves its OWN workspace, not the ambient one', () => {
  it("threads the buffer's own workspaceId prop into XtermTerminal, even when the ambient WorkspaceStoreContext names a DIFFERENT (hidden-copy) workspace", () => {
    receivedProps.length = 0
    // The ambient context belongs to a keep-alive copy of a DIFFERENT workspace
    // ('w-ambient') than the one this terminal's own buffer actually belongs to
    // ('w-owner') — exactly the shape WorkspaceHost produces for a retained
    // background workspace.
    const ambientStore = createWorkspaceStore('w-ambient')

    act(() => {
      render(
        createElement(
          WorkspaceStoreContext.Provider,
          { value: ambientStore },
          createElement(TerminalTab, {
            sessionId: 'pty-owner',
            bufferId: 'buf-owner',
            workspaceId: 'w-owner',
          }),
        ),
      )
    })

    expect(receivedProps).toHaveLength(1)
    expect(receivedProps[0]?.workspaceId).toBe('w-owner')
    expect(receivedProps[0]?.workspaceId).not.toBe('w-ambient')
  })
})
