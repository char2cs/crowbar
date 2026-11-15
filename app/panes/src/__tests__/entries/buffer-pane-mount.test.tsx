import { afterEach, describe, expect, it, vi } from 'vitest'
import type { LifecycleMessage } from '@/sdk/lifecycle-message'
import type { PaneClient } from '@/sdk/pane-client'
import { bufferSession } from '@/entries/buffer-session'

vi.mock('@/entries/buffer-pane-host', () => ({ BufferPaneHost: () => null }))

const { BufferPaneMount } = await import('@/entries/buffer-pane-mount')

function startedMount() {
  let deliver: (message: LifecycleMessage) => void = () => {}
  const client = {
    onLifecycle: (listener: (message: LifecycleMessage) => void) => {
      deliver = listener
      return () => {}
    },
  } as unknown as PaneClient
  new BufferPaneMount(document.createElement('div'), client, () => Promise.resolve(null)).start()
  return (
    event: LifecycleMessage['event'],
    params: Record<string, string> = {},
    state: unknown = null,
  ) => deliver({ paneId: 'p1', event, params, state })
}

afterEach(() => {
  bufferSession.setState({ chatId: null, saved: null, active: false })
})

describe('BufferPaneMount', () => {
  it('shows what the shell saved for the chat named by threadId and follows activation', () => {
    const send = startedMount()
    send('mount', { threadId: 't1', fileId: 'f1' }, { path: 'a.ts' })
    expect(bufferSession.getState()).toEqual({
      chatId: 't1',
      saved: { path: 'a.ts' },
      active: false,
    })
    send('activate')
    expect(bufferSession.getState().active).toBe(true)
    send('deactivate')
    expect(bufferSession.getState().active).toBe(false)
  })

  it.each(['park', 'destroy'] as const)('forgets the pane on %s', (event) => {
    const send = startedMount()
    send('mount', { threadId: 't1' })
    send(event)
    expect(bufferSession.getState().chatId).toBeNull()
  })

  it('refuses a mount without a threadId', () => {
    const send = startedMount()
    expect(() => send('mount')).toThrow(/threadId/)
  })
})
