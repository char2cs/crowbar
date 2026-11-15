import { afterEach, describe, expect, it, vi } from 'vitest'
import type { LifecycleMessage } from '@/sdk/lifecycle-message'
import type { PaneClient } from '@/sdk/pane-client'
import { chatSession } from '@/entries/chat-session'

vi.mock('@/entries/chat-pane-host', () => ({ ChatPaneHost: () => null }))

const { ChatPaneMount } = await import('@/entries/chat-pane-mount')

function startedMount() {
  let deliver: (message: LifecycleMessage) => void = () => {}
  const onLifecycle = (listener: (message: LifecycleMessage) => void) => {
    deliver = listener
    return () => {}
  }
  const client = { onLifecycle } as unknown as PaneClient
  new ChatPaneMount(document.createElement('div'), client).start()
  return (event: LifecycleMessage['event'], params: Record<string, string> = {}) =>
    deliver({ paneId: 'p1', event, params, state: null })
}

afterEach(() => {
  chatSession.setState({ chatId: null, active: false })
})

describe('ChatPaneMount', () => {
  it('shows the chat named by threadId on mount and follows activation', () => {
    const send = startedMount()
    send('mount', { threadId: 't1' })
    expect(chatSession.getState()).toEqual({ chatId: 't1', active: false })
    send('activate')
    expect(chatSession.getState().active).toBe(true)
    send('deactivate')
    expect(chatSession.getState().active).toBe(false)
  })

  it.each(['park', 'destroy'] as const)('forgets the chat on %s', (event) => {
    const send = startedMount()
    send('mount', { threadId: 't1' })
    send(event)
    expect(chatSession.getState().chatId).toBeNull()
  })

  it('refuses a mount without a threadId', () => {
    const send = startedMount()
    expect(() => send('mount')).toThrow(/threadId/)
  })
})
