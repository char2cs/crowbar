import { afterEach, describe, expect, it, vi } from 'vitest'
import * as commands from '@/features/agent/api/agent-chat-commands'
import { offeredStopShell } from '@/features/agent/chat/lib/offered-stop-shell'
import { ApiError } from '@/lib/errors/api-error'
import * as errors from '@/lib/errors/report'

afterEach(() => vi.restoreAllMocks())

describe('offeredStopShell', () => {
  it('is not offered where the provider cannot be asked', () => {
    expect(offeredStopShell('w1', 'c1', false)).toBeUndefined()
  })

  it('asks for the shell by id and reports a refusal instead of throwing it at the page', async () => {
    const gone = new ApiError('not running', 409, 'conflict')
    const stop = vi.spyOn(commands, 'stopShell').mockRejectedValue(gone)
    const reported = vi.spyOn(errors, 'report').mockImplementation(() => {})

    offeredStopShell('w1', 'c1', true)?.('sh-1')

    await vi.waitFor(() =>
      expect(reported).toHaveBeenCalledWith(gone, { action: 'Couldn’t end the shell' }),
    )
    expect(stop).toHaveBeenCalledWith('w1', 'c1', 'sh-1')
  })
})
