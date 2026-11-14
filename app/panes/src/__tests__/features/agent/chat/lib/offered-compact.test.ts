import { afterEach, describe, expect, it, vi } from 'vitest'
import * as commands from '@/features/agent/api/agent-chat-commands'
import { offeredCompact } from '@/features/agent/chat/lib/offered-compact'
import { ApiError } from '@/lib/errors/api-error'
import * as errors from '@/lib/errors/report'

afterEach(() => vi.restoreAllMocks())

describe('offeredCompact', () => {
  it('is not offered while the agent is busy', () => {
    expect(offeredCompact('w1', 'c1', false)).toBeUndefined()
  })

  // A turn can still start between the offer and the click: the refusal says why.
  it('reports a refused compact instead of throwing it at the page', async () => {
    const busy = new ApiError('chat is busy', 409, 'conflict')
    const compact = vi.spyOn(commands, 'compactChat').mockRejectedValue(busy)
    const reported = vi.spyOn(errors, 'report').mockImplementation(() => {})

    offeredCompact('w1', 'c1', true)?.()

    await vi.waitFor(() =>
      expect(reported).toHaveBeenCalledWith(busy, { action: 'Couldn’t compact the chat' }),
    )
    expect(compact).toHaveBeenCalledWith('w1', 'c1')
  })
})
