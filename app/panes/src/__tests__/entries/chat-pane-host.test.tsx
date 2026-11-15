import { render } from '@testing-library/react'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { chatSession } from '@/entries/chat-session'

const chatTitle = vi.hoisted(() => ({ value: '' }))

vi.mock('@/features/agent/hooks/use-chat-entity', () => ({
  useChatField: (_chatId: string, pick: (chat: unknown) => unknown) =>
    pick({ workspaceId: '', title: chatTitle.value }),
}))

const { ChatPaneHost } = await import('@/entries/chat-pane-host')

afterEach(() => {
  chatTitle.value = ''
  chatSession.setState({ chatId: null, active: false })
})

describe('ChatPaneHost', () => {
  it('sends the chat title to the shell tab', () => {
    chatTitle.value = 'Fix pricing rounding'
    const setTitle = vi.fn()
    render(<ChatPaneHost client={{ setTitle }} />)
    expect(setTitle).toHaveBeenCalledWith('Fix pricing rounding')
  })

  it('leaves the shell label alone while the chat is untitled', () => {
    const setTitle = vi.fn()
    render(<ChatPaneHost client={{ setTitle }} />)
    expect(setTitle).not.toHaveBeenCalled()
  })
})
