import { render, screen } from '@testing-library/react'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { chatSession } from '@/entries/chat-session'

const chat = vi.hoisted(() => ({ title: '', workspaceId: '' }))

vi.mock('@/features/agent/hooks/use-chat-entity', () => ({
  useChatField: (_chatId: string, pick: (chat: unknown) => unknown) =>
    pick({ workspaceId: chat.workspaceId, title: chat.title }),
}))

vi.mock('@/features/agent/components/agent-chat-pane-gate', () => ({
  AgentChatPaneGate: ({ belowOverlayHeader }: { belowOverlayHeader?: boolean }) => (
    <div data-testid="gate" data-below={String(belowOverlayHeader)} />
  ),
}))

vi.mock('@/entries/chat-pane-header', () => ({
  ChatPaneHeader: () => <div data-testid="header" />,
}))

const { ChatPaneHost } = await import('@/entries/chat-pane-host')

afterEach(() => {
  chat.title = ''
  chat.workspaceId = ''
  chatSession.setState({ chatId: null, active: false })
})

describe('ChatPaneHost', () => {
  it.each([
    ['sends the chat title to the shell tab', 'Fix pricing rounding', 1],
    ['leaves the shell label alone while the chat is untitled', '', 0],
  ])('%s', (_name, title, sent) => {
    chat.title = title
    const setTitle = vi.fn()
    render(<ChatPaneHost client={{ setTitle }} />)
    expect(setTitle).toHaveBeenCalledTimes(sent)
  })

  it('clears the chat surface below the identity row', () => {
    chat.workspaceId = 'ws-1'
    chatSession.setState({ chatId: 'chat-1', active: true })
    render(<ChatPaneHost client={{ setTitle: vi.fn() }} />)
    expect(screen.getByTestId('header')).toBeTruthy()
    expect(screen.getByTestId('gate').dataset.below).toBe('true')
  })
})
