import { beforeEach, describe, expect, it } from 'vitest'
import { renderHook } from '@testing-library/react'
import { useChatTitle } from '@/features/agent/hooks/use-chat-title'
import { UNTITLED_CHAT_LABEL } from '@/features/agent/lib/chat-label'
import { chatEntity, putEntities, resetEntities } from '@/__tests__/__fixtures__/sidebar-entities'

const titleOf = (chatId: string) => renderHook(() => useChatTitle(chatId)).result.current

beforeEach(() => resetEntities())

describe('useChatTitle', () => {
  it("answers the record's own title", () => {
    putEntities('chat', [chatEntity({ id: 'chat-1', title: 'repochat-RN' })])
    expect(titleOf('chat-1')).toBe('repochat-RN')
  })

  it('answers UNTITLED_CHAT_LABEL for a record that is here and has no title', () => {
    putEntities('chat', [chatEntity({ id: 'chat-1', title: '' })])
    expect(titleOf('chat-1')).toBe(UNTITLED_CHAT_LABEL)
  })

  // The distinction the pane header lost: an absent record used to collapse
  // into UNTITLED_CHAT_LABEL, so a store that had simply not got the chat
  // rendered identically to a chat nobody had named.
  it('answers null — not UNTITLED_CHAT_LABEL — when no record is held', () => {
    expect(titleOf('chat-1')).toBeNull()
  })
})
