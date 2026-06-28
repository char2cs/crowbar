import { describe, it, expect, vi, beforeEach } from 'vitest'

vi.mock('@/lib/crowbar-bridge', () => ({ terminalDetach: vi.fn(async () => {}) }))

import { detachTerminalSession } from '@/features/terminal/lib/detach-terminal-session'
import { useTerminalStore } from '@/features/terminal/stores/terminal-store'
import { loadReconnect } from '@/features/terminal/lib/terminal-reconnect-map'
import { terminalDetach } from '@/lib/crowbar-bridge'

beforeEach(() => {
  vi.clearAllMocks()
  localStorage.clear()
  useTerminalStore.setState({ sessions: new Map() })
})

describe('detachTerminalSession', () => {
  it('detaches by connectionId, persists the mapping, and keeps the store entry', async () => {
    useTerminalStore.getState().updateSession('tab-1', { connectionId: 'conn-1' })

    await detachTerminalSession('ws-1', 'tab-1')

    expect(terminalDetach).toHaveBeenCalledWith('conn-1')            // by connectionId, not tab id
    expect(loadReconnect('ws-1', 'tab-1')).toBe('conn-1')           // mapping persisted
    expect(useTerminalStore.getState().getSession('tab-1')).toBeTruthy() // store entry kept
  })

  it('no-ops when there is no connectionId', async () => {
    await detachTerminalSession('ws-1', 'tab-unknown')
    expect(terminalDetach).not.toHaveBeenCalled()
  })
})
