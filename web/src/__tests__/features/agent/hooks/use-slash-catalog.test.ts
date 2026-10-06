import { act, renderHook, waitFor } from '@testing-library/react'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { ApiError } from '@/lib/api'
import { useSlashCatalog } from '@/features/agent/hooks/use-slash-catalog'

const getSlashCatalog = vi.fn()

vi.mock('@/features/agent/api/agent-api', () => ({
  getSlashCatalog: (...args: unknown[]) => getSlashCatalog(...args),
}))

const catalog = { providerId: 'claude', completeness: 'partial', items: [] }

type Props = { active: boolean; providerId?: string }

function mount(initialProps: Props) {
  return renderHook(
    ({ active, providerId = 'claude' }: Props) =>
      useSlashCatalog({ wsId: 'ws1', chatId: 'c1', providerId, active, draft: '' }),
    { initialProps },
  )
}

beforeEach(() => {
  getSlashCatalog.mockReset()
  getSlashCatalog.mockImplementation((_ws: string, _chat: string, _signal: AbortSignal) =>
    Promise.resolve(catalog),
  )
})

describe('useSlashCatalog presaving', () => {
  it('probes nothing while the chat is not active', () => {
    mount({ active: false })

    expect(getSlashCatalog).not.toHaveBeenCalled()
  })

  it('probes once when first shown and keeps the catalogue across hide and show', async () => {
    const { result, rerender } = mount({ active: true })
    await waitFor(() => expect(result.current.state.state).toBe('ready'))

    rerender({ active: false })
    rerender({ active: true })
    await act(async () => {})

    expect(getSlashCatalog).toHaveBeenCalledTimes(1)
    expect(result.current.state.state).toBe('ready')
  })

  it('asks again after a provider switch, since those are another CLI skills', async () => {
    const { result, rerender } = mount({ active: true })
    await waitFor(() => expect(result.current.state.state).toBe('ready'))
    getSlashCatalog.mockResolvedValue({ ...catalog, providerId: 'codex' })

    rerender({ active: true, providerId: 'codex' })

    await waitFor(() => expect(getSlashCatalog).toHaveBeenCalledTimes(2))
  })

  it('retries a failed probe the next time the chat is shown', async () => {
    getSlashCatalog.mockRejectedValueOnce(new Error('probe failed'))
    const { result, rerender } = mount({ active: true })
    await waitFor(() => expect(result.current.state.state).toBe('error'))

    rerender({ active: false })
    rerender({ active: true })

    await waitFor(() => expect(result.current.state.state).toBe('ready'))
    expect(getSlashCatalog).toHaveBeenCalledTimes(2)
  })

  // 422 is the daemon saying this provider has no catalogue to ask for: a stable
  // answer, so showing the chat again must not put the same question again.
  it('does not re-ask a provider the daemon declared has no catalogue', async () => {
    getSlashCatalog.mockRejectedValue(new ApiError('unavailable', 422))
    const { result, rerender } = mount({ active: true })
    await waitFor(() => expect(result.current.state.state).toBe('error'))

    rerender({ active: false })
    rerender({ active: true })
    await act(async () => {})

    expect(getSlashCatalog).toHaveBeenCalledTimes(1)
  })
})
