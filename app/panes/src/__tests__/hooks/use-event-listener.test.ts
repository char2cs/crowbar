import { renderHook } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'
import { useEventListener } from '@/hooks/use-event-listener'

describe('useEventListener', () => {
  it('calls the latest handler and unsubscribes on unmount', () => {
    const target = { current: document }
    const first = vi.fn()
    const second = vi.fn()
    const { rerender, unmount } = renderHook(
      ({ handler }) => useEventListener('keydown', handler, target),
      { initialProps: { handler: first } },
    )

    rerender({ handler: second })
    document.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape' }))
    expect(first).not.toHaveBeenCalled()
    expect(second).toHaveBeenCalledTimes(1)

    unmount()
    document.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape' }))
    expect(second).toHaveBeenCalledTimes(1)
  })
})
