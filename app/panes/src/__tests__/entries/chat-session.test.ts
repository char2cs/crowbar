import { describe, expect, it } from 'vitest'
import { chatSession } from '@/entries/chat-session'

describe('chatSession', () => {
  it('starts with no chat and an inactive pane, and holds what it is told', () => {
    expect(chatSession.getState()).toEqual({ chatId: null, active: false })
    chatSession.setState({ chatId: 'c1', active: true })
    expect(chatSession.getState()).toEqual({ chatId: 'c1', active: true })
    chatSession.setState({ chatId: null, active: false })
  })
})
