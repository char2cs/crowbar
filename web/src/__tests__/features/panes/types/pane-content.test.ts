import { describe, it, expect } from 'vitest'
import {
  isAgentChatContent,
  isBranchReviewContent,
  isVirtualContent,
  type AgentChatContent,
  type PaneContent,
} from '@/features/panes/types/pane-content'

function makeAgentChatContent(overrides: Partial<AgentChatContent> = {}): AgentChatContent {
  return {
    id: 'buf-1',
    type: 'agentChat',
    chatId: 'chat-1',
    wsId: 'ws-1',
    path: 'agent-chat://chat-1',
    name: 'Chat 1',
    isPinned: false,
    isPreview: false,
    isActive: false,
    ...overrides,
  }
}

function makeBranchReviewContent(): PaneContent {
  return {
    id: 'buf-2',
    type: 'branchReview',
    wsId: 'ws-1',
    path: 'branch-review://ws-1',
    name: 'Review',
    isPinned: false,
    isPreview: false,
    isActive: false,
  }
}

describe('pane-content agentChat', () => {
  it('isAgentChatContent returns true for agentChat content', () => {
    const content = makeAgentChatContent()
    expect(isAgentChatContent(content)).toBe(true)
  })

  it('isAgentChatContent returns false for other content types', () => {
    const content = makeBranchReviewContent()
    expect(isAgentChatContent(content)).toBe(false)
  })

  it('isAgentChatContent narrows the discriminated union so chatId/wsId are accessible', () => {
    const content: PaneContent = makeAgentChatContent({ chatId: 'chat-42', wsId: 'ws-9' })
    if (isAgentChatContent(content)) {
      // Compile-time narrowing check: these fields only exist on AgentChatContent.
      expect(content.chatId).toBe('chat-42')
      expect(content.wsId).toBe('ws-9')
    } else {
      throw new Error('expected agentChat content to narrow')
    }
  })

  it('does not misfire isBranchReviewContent for agentChat content', () => {
    const content = makeAgentChatContent()
    expect(isBranchReviewContent(content)).toBe(false)
  })

  it('treats agentChat as virtual content (not backed by a real file on disk)', () => {
    const content = makeAgentChatContent()
    expect(isVirtualContent(content)).toBe(true)
  })
})
