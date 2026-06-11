import { vi, test, expect } from 'vitest'
import { render } from '@testing-library/react'

// MarkdownChatView has complex deps — stub it out
vi.mock('@/features/markdown-chat/components/markdown-chat-view', () => ({
  MarkdownChatView: ({ chatId }: { chatId: string }) => (
    <div data-testid="markdown-chat-view" data-chat-id={chatId} />
  ),
}))

import { ChatPage } from '@/features/markdown-chat/components/chat-page'

test('ChatPage renders MarkdownChatView with the chat id', () => {
  const { getByTestId } = render(<ChatPage chatId="c1" />)
  const view = getByTestId('markdown-chat-view')
  expect(view).toBeTruthy()
  expect(view.getAttribute('data-chat-id')).toBe('c1')
})
