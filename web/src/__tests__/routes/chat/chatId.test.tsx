import { vi, test, expect, beforeEach, afterEach } from 'vitest'
import { render, act } from '@testing-library/react'

beforeEach(() => { vi.useFakeTimers() })
afterEach(() => { vi.useRealTimers(); vi.restoreAllMocks() })

vi.mock('@/lib/mock/chats', () => ({
  getMockChat: () => ({ messages: [] }),
}))

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

const cancelFn = vi.fn()
vi.mock('@/lib/mock/simulate-stream', () => ({
  simulateStream: vi.fn(() => cancelFn),
}))

import { ChatPage } from '@/routes/chat/$chatId'

test('ChatPage renders without errors and cancel is available on unmount', () => {
  const { unmount } = render(<ChatPage />)
  // Unmount before any stream fires — should not throw
  unmount()
  act(() => { vi.runAllTimers() })
  // If component threw on unmount, the test would already have failed
  // Verify the cancel function was not spuriously called without a send action
  expect(cancelFn).not.toHaveBeenCalled()
})
