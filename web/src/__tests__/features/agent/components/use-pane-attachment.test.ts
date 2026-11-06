import { renderHook } from '@testing-library/react'
import { afterEach, describe, expect, it, vi } from 'vitest'
import * as commandsApi from '@/features/agent/api/agent-chat-commands'
import { usePaneAttachment } from '@/features/agent/components/use-pane-attachment'
import { resetChatStore, seedChats } from '@/__tests__/__fixtures__/agent-chat'
import { ledger, seedFeed } from '@/__tests__/__fixtures__/ledger-rows'

function attachmentOn(chatId: string, providerName = 'Claude') {
  return renderHook(() =>
    usePaneAttachment({
      wsId: 'w1',
      chatId,
      providerName,
      presentation: 'terminal',
      promptReplacing: false,
    }),
  ).result
}

describe('usePaneAttachment', () => {
  afterEach(() => {
    resetChatStore()
    vi.restoreAllMocks()
  })

  it('reads a new chat no CLI has run on as not yet started, not as an ended session', () => {
    seedChats([{ id: 'c1' }])
    seedFeed('c1')

    const result = attachmentOn('c1')

    expect(result.current.attachment).toEqual({
      state: 'idle',
      message: 'Your first message starts Claude.',
    })
    expect(result.current.canSend).toBe(true)
  })

  // A chat with no provider chosen yet names "the agent", which must not open a sentence.
  it('names an unchosen provider mid-sentence in the not-yet-started line', () => {
    seedChats([{ id: 'c1' }])
    seedFeed('c1')

    expect(attachmentOn('c1', 'the agent').current.attachment).toEqual({
      state: 'idle',
      message: 'Your first message starts the agent.',
    })
  })

  it('starts a new chat through the same resume a dormant one takes', () => {
    const resume = vi.spyOn(commandsApi, 'resumeChat').mockResolvedValue('r1')
    seedChats([{ id: 'c1' }])
    seedFeed('c1')

    attachmentOn('c1').current.startSession()

    expect(resume).toHaveBeenCalledWith('w1', 'c1')
  })

  it('says why a chat that ran is dormant', () => {
    const l = ledger('c1')
    seedChats([{ id: 'c1' }])
    seedFeed('c1', [l.row('runner_exited', { runnerId: 'r1', reason: 'stopped', exitCode: 0 })])

    expect(attachmentOn('c1').current.attachment).toEqual({
      state: 'idle',
      message: 'Stopped. Send a message to continue the conversation.',
    })
  })
})
