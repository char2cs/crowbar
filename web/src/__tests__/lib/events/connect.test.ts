import '@/lib/transport/polyfill'
import { connectDaemonEvents } from '@/lib/events/connect'
import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'

const mockFetch = vi.fn().mockResolvedValue(undefined)

vi.mock('@/lib/store/workspace-list', () => ({
  useWorkspaceListStore: {
    // eslint-disable-next-line @typescript-eslint/no-explicit-any
    getState: () => ({ fetch: mockFetch }) as any,
  },
}))

describe('connectDaemonEvents', () => {
  let disconnect: () => void

  beforeEach(() => {
    mockFetch.mockClear()
    disconnect = connectDaemonEvents()
  })

  afterEach(() => {
    disconnect()
  })

  it('calls workspace list fetch on workspace:updated', () => {
    window.__CROWBAR__.emit('workspace:updated')
    expect(mockFetch).toHaveBeenCalled()
  })

  it('dispatches git-status-changed CustomEvent on git:changed', () => {
    const handler = vi.fn()
    window.addEventListener('git-status-changed', handler)
    window.__CROWBAR__.emit('git:changed', { workspaceId: 'ws-1' })
    expect(handler).toHaveBeenCalled()
    window.removeEventListener('git-status-changed', handler)
  })

  it('dispatches file-external-change CustomEvent on file:changed', () => {
    const handler = vi.fn()
    window.addEventListener('file-external-change', handler)
    window.__CROWBAR__.emit('file:changed', { workspaceId: 'ws-1', path: '/src/foo.ts' })
    expect(handler).toHaveBeenCalledOnce()
    const event = handler.mock.calls[0][0] as CustomEvent
    expect(event.detail).toMatchObject({ event_type: 'modify', path: '/src/foo.ts' })
    window.removeEventListener('file-external-change', handler)
  })
})
