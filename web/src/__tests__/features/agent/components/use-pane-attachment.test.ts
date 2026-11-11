import { act, renderHook } from '@testing-library/react'
import { afterEach, describe, expect, it, vi } from 'vitest'
import * as commandsApi from '@/features/agent/api/agent-chat-commands'
import { usePaneAttachment } from '@/features/agent/components/use-pane-attachment'
import { resetChatStore, seedChats } from '@/__tests__/__fixtures__/agent-chat'
import { ledger, seedFeed } from '@/__tests__/__fixtures__/ledger-rows'
import { pairRemoteHost, resetHostWorld } from '@/__tests__/__fixtures__/remote-host'
import {
  destroyHostRuntime,
  getOrCreateHostRuntime,
} from '@/features/hosts/stores/host-runtime-registry'
import { useConnectionStore } from '@/lib/ws/connection-store'

function attachmentOn(
  chatId: string,
  {
    providerName = 'Claude',
    presentation = 'terminal',
    hostId = 'local',
  }: { providerName?: string; presentation?: 'chat' | 'terminal'; hostId?: string } = {},
) {
  return renderHook(() =>
    usePaneAttachment({
      wsId: 'w1',
      hostId,
      chatId,
      providerName,
      presentation,
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

    expect(attachmentOn('c1', { providerName: 'the agent' }).current.attachment).toEqual({
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

  it('says the chat is connecting while its live CLI moves onto the chat surface', () => {
    seedChats([{ id: 'c1', liveRunnerId: 'r1', phase: 'switching' }])
    seedFeed('c1')

    expect(attachmentOn('c1', { presentation: 'chat' }).current.connecting).toBe(
      'Connecting to Claude…',
    )
  })

  it('says it is connecting while a dormant chat with history starts its CLI', () => {
    seedChats([{ id: 'c1', liveRunnerId: '', phase: 'starting' }])
    seedFeed('c1')

    expect(attachmentOn('c1', { presentation: 'chat' }).current.connecting).toBe(
      'Connecting to Claude…',
    )
  })

  it('says nothing about connecting once the switch has landed', () => {
    seedChats([{ id: 'c1', liveRunnerId: 'r1', phase: 'live' }])
    seedFeed('c1')

    expect(attachmentOn('c1', { presentation: 'chat' }).current.connecting).toBeUndefined()
  })

  describe('when the chat’s host does not answer', () => {
    const downHost = () => {
      pairRemoteHost('h1')
      getOrCreateHostRuntime('h1').getState().setStatus('down')
      seedChats([{ id: 'c1' }])
      seedFeed('c1')
    }
    afterEach(() => {
      resetHostWorld()
      destroyHostRuntime('h1')
      useConnectionStore.setState({ status: 'connected' })
    })

    it('cannot send, and is offline, until the host is back up', () => {
      downHost()
      const result = attachmentOn('c1', { hostId: 'h1' })
      expect([result.current.offline, result.current.canSend]).toEqual([true, false])
      act(() => getOrCreateHostRuntime('h1').getState().setStatus('up'))
      expect([result.current.offline, result.current.canSend]).toEqual([false, true])
    })

    it('is offline when the local daemon’s socket is down', () => {
      useConnectionStore.setState({ status: 'disconnected' })
      seedChats([{ id: 'c1' }])
      expect(attachmentOn('c1').current.offline).toBe(true)
    })

    it('starts no session on it', () => {
      const resume = vi.spyOn(commandsApi, 'resumeChat').mockResolvedValue('r1')
      downHost()
      attachmentOn('c1', { hostId: 'h1' }).current.startSession()
      expect(resume).not.toHaveBeenCalled()
    })
  })

  // Every way onto the terminal may call it: only a known chat that is dormant now is started.
  it('starts a session from the chat as it is when called, never a live or unknown one', () => {
    const resume = vi.spyOn(commandsApi, 'resumeChat').mockResolvedValue('r1')
    seedChats([{ id: 'c1' }])
    const stale = attachmentOn('c1').current.startSession
    act(() => seedChats([{ id: 'c1', liveRunnerId: 'r1' }]))
    stale()
    attachmentOn('c9').current.startSession()
    expect(resume).not.toHaveBeenCalled()
  })
})
