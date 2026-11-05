import { createElement } from 'react'
import { act, render } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'
import { WorkspaceStoreContext } from '@/features/workspace/stores/workspace-context'
import { createWorkspaceStore } from '@/features/workspace/stores/workspace-store'

// TerminalTab passes its own buffer's workspaceId through, ignoring the ambient context: under
// another workspace's context, a reconnect would otherwise spawn its PTY in the wrong worktree.
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
