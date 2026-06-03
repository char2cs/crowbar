import { vi, test, expect } from 'vitest'
import { render } from '@testing-library/react'

vi.mock('@tanstack/react-router', async (importOriginal) => {
  const actual = await importOriginal<typeof import('@tanstack/react-router')>()
  return {
    ...actual,
    createFileRoute: () => () => ({
      component: null,
      useParams: () => ({ chatId: 'c1' }),
    }),
  }
})

// MarkdownChatView has complex deps — stub it out
vi.mock('@/features/markdown-chat/components/markdown-chat-view', () => ({
  MarkdownChatView: ({ workspaceId, stepId }: { workspaceId: string; stepId: string }) => (
    <div data-testid="markdown-chat-view" data-workspace-id={workspaceId} data-step-id={stepId} />
  ),
}))

import { ChatPage } from '@/routes/chat/$chatId'

test('ChatPage renders MarkdownChatView with chatId as workspaceId', () => {
  const { getByTestId } = render(<ChatPage />)
  const view = getByTestId('markdown-chat-view')
  expect(view).toBeTruthy()
  expect(view.getAttribute('data-workspace-id')).toBe('c1')
  expect(view.getAttribute('data-step-id')).toBe('chat')
})
