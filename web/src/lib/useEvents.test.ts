import { describe, it, expect, vi, afterEach } from 'vitest'
import { renderHook } from '@testing-library/react'
import { useEvents } from './useEvents'

describe('useEvents', () => {
  afterEach(() => vi.restoreAllMocks())

  it('opens an EventSource on mount', () => {
    const mockES = {
      addEventListener: vi.fn(),
      removeEventListener: vi.fn(),
      close: vi.fn(),
      readyState: 0,
    }
    vi.stubGlobal('EventSource', vi.fn(() => mockES))

    renderHook(() => useEvents('message', vi.fn()))

    expect(EventSource).toHaveBeenCalledOnce()
  })

  it('closes EventSource on unmount', () => {
    const mockES = {
      addEventListener: vi.fn(),
      removeEventListener: vi.fn(),
      close: vi.fn(),
      readyState: 0,
    }
    vi.stubGlobal('EventSource', vi.fn(() => mockES))

    const { unmount } = renderHook(() => useEvents('message', vi.fn()))
    unmount()

    expect(mockES.close).toHaveBeenCalledOnce()
  })
})
