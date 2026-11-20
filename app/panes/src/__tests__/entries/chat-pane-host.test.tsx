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
  ChatPaneHeader: ({ onOpenEditor }: { onOpenEditor?: () => void }) => (
    <div data-testid="header">
      {onOpenEditor && <button onClick={onOpenEditor}>Open editor</button>}
    </div>
  ),
}))

const { ChatPaneHost } = await import('@/entries/chat-pane-host')

afterEach(() => {
  chat.title = ''
  chat.workspaceId = ''
  chatSession.setState({ chatId: null, active: false, fillsGroup: false })
})

describe('ChatPaneHost', () => {
  it.each([
    ['sends the chat title to the shell tab', 'Fix pricing rounding', 1],
    ['leaves the shell label alone while the chat is untitled', '', 0],
  ])('%s', (_name, title, sent) => {
    chat.title = title
    const setTitle = vi.fn()
    render(<ChatPaneHost client={{ setTitle, runCommand: vi.fn() }} />)
    expect(setTitle).toHaveBeenCalledTimes(sent)
  })

  it('clears the chat surface below the identity row', () => {
    chat.workspaceId = 'ws-1'
    chatSession.setState({ chatId: 'chat-1', active: true })
    render(<ChatPaneHost client={{ setTitle: vi.fn(), runCommand: vi.fn() }} />)
    expect(screen.getByTestId('header')).toBeTruthy()
    expect(screen.getByTestId('gate').dataset.below).toBe('true')
  })

  it.each([
    ['asks the shell to open the editor while the chat fills its group', true, 1],
    ['offers nothing while the editor is beside it', false, 0],
  ])('%s', (_name, fillsGroup, buttons) => {
    chat.workspaceId = 'ws-1'
    chatSession.setState({ chatId: 'chat-1', active: true, fillsGroup })
    const runCommand = vi.fn()
    render(<ChatPaneHost client={{ setTitle: vi.fn(), runCommand }} />)
    const offered = screen.queryAllByRole('button', { name: 'Open editor' })
    expect(offered).toHaveLength(buttons)
    offered.forEach((button) => button.click())
    expect(runCommand.mock.calls).toEqual(buttons ? [['panes.toggleEditor']] : [])
  })
})
