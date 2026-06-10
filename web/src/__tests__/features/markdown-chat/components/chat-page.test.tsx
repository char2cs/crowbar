import { vi, test, expect } from 'vitest'
import { render } from '@testing-library/react'

// MarkdownChatView has complex deps — stub it out
vi.mock('@/features/markdown-chat/components/markdown-chat-view', () => ({
  MarkdownChatView: ({ workspaceId, stepId }: { workspaceId: string; stepId: string }) => (
    <div data-testid="markdown-chat-view" data-workspace-id={workspaceId} data-step-id={stepId} />
  ),
}))

import { ChatPage } from '@/features/markdown-chat/components/chat-page'

test('ChatPage renders MarkdownChatView with chatId as workspaceId', () => {
  const { getByTestId } = render(<ChatPage chatId="c1" />)
  const view = getByTestId('markdown-chat-view')
  expect(view).toBeTruthy()
  expect(view.getAttribute('data-workspace-id')).toBe('c1')
  expect(view.getAttribute('data-step-id')).toBe('chat')
})
