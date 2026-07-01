import { describe, it, expect, beforeEach, vi } from 'vitest'
import { killTerminalSession } from '@/features/terminal/lib/kill-terminal-session'
import { useTerminalStore } from '@/features/terminal/stores/terminal-store'
import { terminalClose } from '@/lib/crowbar-bridge'

vi.mock('@/lib/crowbar-bridge', () => ({
  terminalClose: vi.fn(async () => {}),
}))

beforeEach(() => {
  vi.clearAllMocks()
  useTerminalStore.setState({ sessions: new Map() })
})

describe('killTerminalSession', () => {
  // BUG-015: closing a terminal tab must terminate the backend PTY (DELETE
  // /v0/terminals/:sessionId via terminalClose) — otherwise shells leak.
  it('closes the backend connection for the session and drops the store entry', async () => {
    useTerminalStore.getState().updateSession('tab-1', { connectionId: 'pty-42' })

    await killTerminalSession('tab-1')

    expect(terminalClose).toHaveBeenCalledWith('pty-42')
    expect(useTerminalStore.getState().getSession('tab-1')).toBeUndefined()
  })

  it('is a no-op on the backend when the session never connected', async () => {
    useTerminalStore.getState().updateSession('tab-2', {})

    await killTerminalSession('tab-2')

    expect(terminalClose).not.toHaveBeenCalled()
    expect(useTerminalStore.getState().getSession('tab-2')).toBeUndefined()
  })

  it('handles unknown session ids without calling the backend', async () => {
    await killTerminalSession('missing')
    expect(terminalClose).not.toHaveBeenCalled()
  })
})
