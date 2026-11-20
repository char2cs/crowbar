import { describe, expect, it } from 'vitest'
import { chatSession } from '@/entries/chat-session'

describe('chatSession', () => {
  it('starts with no chat, an inactive pane and no group, and holds what it is told', () => {
    expect(chatSession.getState()).toEqual({ chatId: null, active: false, fillsGroup: false })
    chatSession.setState({ chatId: 'c1', active: true, fillsGroup: true })
    expect(chatSession.getState()).toEqual({ chatId: 'c1', active: true, fillsGroup: true })
    chatSession.setState({ chatId: null, active: false, fillsGroup: false })
  })
})
