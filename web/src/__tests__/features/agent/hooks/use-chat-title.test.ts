import { describe, expect, it } from 'vitest'
import type { AgentChat } from '@/features/agent/api/agent-api'
import { chatTitleIn } from '@/features/agent/hooks/use-chat-title'
import { UNTITLED_CHAT_LABEL } from '@/features/agent/lib/chat-label'

function makeChat(overrides: Partial<AgentChat> = {}): AgentChat {
  return {
    id: 'chat-1',
    workspaceId: 'w1',
    title: 'repochat-RN',
    liveRunnerId: '',
    terminalSessionId: '',
    activeProviderId: '',
    createdAt: new Date().toISOString(),
    parentId: '',
    ...overrides,
  } as AgentChat
}

describe('chatTitleIn', () => {
  it("answers the record's own title", () => {
    expect(chatTitleIn([makeChat()], 'chat-1')).toBe('repochat-RN')
  })

  it('answers UNTITLED_CHAT_LABEL for a record that is here and has no title', () => {
    expect(chatTitleIn([makeChat({ title: '' })], 'chat-1')).toBe(UNTITLED_CHAT_LABEL)
  })

  // The distinction the pane header lost: an absent record used to collapse
  // into UNTITLED_CHAT_LABEL, so a store that had simply not got the chat
  // rendered identically to a chat nobody had named.
  it('answers null — not UNTITLED_CHAT_LABEL — when no record for the id is present', () => {
    expect(chatTitleIn([makeChat({ id: 'other' })], 'chat-1')).toBeNull()
    expect(chatTitleIn([], 'chat-1')).toBeNull()
  })
})
