import { render } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'
import type { AgentChatMessage, AgentProvider } from '@/features/agent/api/agent-api'
import { MessageRow } from '@/features/agent/transcript/message-row'

const markdownRenderCount = vi.hoisted(() => ({ count: 0 }))

// Both mocked, sharing one counter: these tests assert on MessageRow's own
// memoization, not on which of the two markdown renderers a given row picks
// (message-row.test.tsx covers that split) — every message here is settled
// (streaming defaults to false), so MarkdownMessageStatic is the one that
// actually renders.
vi.mock('@/features/agent/transcript/plate/markdown-message', () => ({
  MarkdownMessage: ({ children }: { children: string }) => {
    markdownRenderCount.count++
    return <span>{children}</span>
  },
}))

vi.mock('@/features/agent/transcript/plate/markdown-message-static', () => ({
  MarkdownMessageStatic: ({ children }: { children: string }) => {
    markdownRenderCount.count++
    return <span>{children}</span>
  },
}))

describe('MessageRow memo (render-count proof)', () => {
  it('does not re-render its subtree when called twice with identical prop references', () => {
    markdownRenderCount.count = 0
    const providers: AgentProvider[] = []
    const message: AgentChatMessage = {
      turnId: 't1',
      sequence: 1,
      role: 'assistant',
      providerId: 'claude',
      text: 'hi',
      at: '2026-08-24T00:00:00Z',
    }
    const { rerender } = render(<MessageRow message={message} providers={providers} />)
    expect(markdownRenderCount.count).toBe(1)

    rerender(<MessageRow message={message} providers={providers} />)
    expect(markdownRenderCount.count).toBe(1) // still 1 — memo skipped the second render entirely
  })

  it('DOES re-render when a prop reference changes', () => {
    markdownRenderCount.count = 0
    const providers: AgentProvider[] = []
    const message: AgentChatMessage = {
      turnId: 't1',
      sequence: 1,
      role: 'assistant',
      providerId: 'claude',
      text: 'hi',
      at: '2026-08-24T00:00:00Z',
    }
    const { rerender } = render(<MessageRow message={message} providers={providers} />)
    expect(markdownRenderCount.count).toBe(1)

    const changedMessage = { ...message, text: 'changed' }
    rerender(<MessageRow message={changedMessage} providers={providers} />)
    expect(markdownRenderCount.count).toBe(2)
  })
})
