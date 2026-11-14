import { act, renderHook } from '@testing-library/react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { ledger, seedFeed } from '@/__tests__/__fixtures__/ledger-rows'
import { resetChatStore } from '@/__tests__/__fixtures__/agent-chat'
import * as commands from '@/features/agent/api/agent-chat-commands'
import { useChatSend } from '@/features/agent/chat/use-chat-send'
import { ApiError } from '@/lib/errors/api-error'
import { toastManager } from '@/lib/toast-manager'

const withProvider = new ApiError('agent: retract: with its provider', 409, 'prompt_with_provider')

beforeEach(() => {
  resetChatStore()
  vi.spyOn(toastManager, 'add').mockReturnValue('')
})
afterEach(() => vi.restoreAllMocks())

// Stop before a prompt's turn runs: the prompt comes back, or, once its provider
// has it and nothing can take it back, the turn it starts is stopped instead.
describe('useChatSend — withdraw', () => {
  const held = () => {
    const l = ledger('c1')
    seedFeed('c1', [l.prompt('r0', 'earlier'), l.start('t0', 'r0'), l.end('t0')])
    seedFeed('c1', [l.prompt('r1', 'never mind')])
    return renderHook(() => useChatSend('w1', 'c1', false, () => {})).result
  }

  it('takes back a prompt its provider does not have yet, and stops nothing', async () => {
    vi.spyOn(commands, 'retractPrompt').mockResolvedValue({
      state: 'retracted',
      text: 'never mind',
    })
    const stop = vi.spyOn(commands, 'stopChat').mockResolvedValue()
    const send = held()

    await act(async () => expect(await send.current.withdraw()).toBe('never mind'))

    expect(stop).not.toHaveBeenCalled()
  })

  it('stops the chat when the provider already has the prompt', async () => {
    vi.spyOn(commands, 'retractPrompt').mockRejectedValue(withProvider)
    const stop = vi.spyOn(commands, 'stopChat').mockResolvedValue()
    const send = held()

    await act(async () => expect(await send.current.withdraw()).toBeNull())

    expect(stop).toHaveBeenCalledWith('w1', 'c1')
  })
})
