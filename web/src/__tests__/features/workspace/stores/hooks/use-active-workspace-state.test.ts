import { describe, it, expect, vi, beforeEach } from 'vitest'
import { renderHook, act } from '@testing-library/react'
import { useActiveWorkspaceState } from '@/features/workspace/stores/hooks/use-active-workspace-state'

// Captured callbacks from mocks so tests can trigger events manually.
let capturedChangeListener: ((store: unknown) => void) | null = null
let capturedStoreSubscriber: ((state: unknown) => void) | null = null

const mockGetState = vi.fn()

const mockStore = {
  getState: mockGetState,
  subscribe: vi.fn((fn: (state: unknown) => void) => {
    capturedStoreSubscriber = fn
    return () => {
      capturedStoreSubscriber = null
    }
  }),
}

// Controls whether the mock store ref returns a store or null.
let activeStoreOverride: typeof mockStore | null = mockStore

vi.mock('@/features/workspace/stores/workspace-store-ref', () => ({
  getActiveWorkspaceStoreRef: vi.fn(() => activeStoreOverride),
  onActiveWorkspaceStoreChange: vi.fn((listener: (store: unknown) => void) => {
    capturedChangeListener = listener
    // Fire immediately with the current store (documented behavior).
    listener(activeStoreOverride)
    return () => {
      capturedChangeListener = null
    }
  }),
}))

beforeEach(() => {
  capturedChangeListener = null
  capturedStoreSubscriber = null
  activeStoreOverride = mockStore
  mockGetState.mockReset()
  ;(mockStore.subscribe as ReturnType<typeof vi.fn>).mockClear()
})

describe('useActiveWorkspaceState', () => {
  it('returns the initial value derived from the active store', () => {
    mockGetState.mockReturnValue({ key: 'hello' })

    const { result } = renderHook(() =>
      useActiveWorkspaceState((s) => (s as unknown as { key: string }).key, 'default'),
    )

    expect(result.current).toBe('hello')
  })

  it('returns fallback when no store is active', () => {
    activeStoreOverride = null

    const { result } = renderHook(() =>
      useActiveWorkspaceState((_s: unknown) => 'should-not-be-used', 'my-fallback'),
    )

    expect(result.current).toBe('my-fallback')
  })

  it('uses the latest selector after a parent re-render (stale closure fix)', () => {
    // State has both keys; initial selector reads 'key'.
    const storeState = { key: 'a', other: 'b' }
    mockGetState.mockReturnValue(storeState)

    const { result, rerender } = renderHook(
      ({ pick }: { pick: 'key' | 'other' }) =>
        useActiveWorkspaceState((s) => (s as unknown as typeof storeState)[pick], 'fallback'),
      { initialProps: { pick: 'key' as 'key' | 'other' } },
    )

    // Initial render: selector reads 'key' → 'a'.
    expect(result.current).toBe('a')

    // Re-render with a NEW selector that reads 'other'.
    rerender({ pick: 'other' })

    // Trigger a store update. Without the ref fix the subscription closure
    // would still call the OLD selector ('key' → 'a'). With refs it reads
    // selectorRef.current which is now the new selector ('other' → 'b').
    act(() => {
      capturedStoreSubscriber?.(storeState)
    })

    expect(result.current).toBe('b')
  })

  it('re-subscribes and reads from the new store when the active workspace changes', () => {
    const initialState = { key: 'first' }
    mockGetState.mockReturnValue(initialState)

    const { result } = renderHook(() =>
      useActiveWorkspaceState((s) => (s as unknown as typeof initialState).key, 'fallback'),
    )

    expect(result.current).toBe('first')

    // Simulate the active workspace store being swapped.
    const nextState = { key: 'second' }
    const nextStore = {
      getState: vi.fn(() => nextState),
      subscribe: vi.fn((fn: (state: unknown) => void) => {
        capturedStoreSubscriber = fn
        return () => {}
      }),
    }

    act(() => {
      capturedChangeListener?.(nextStore)
    })

    expect(result.current).toBe('second')
  })

  it('returns fallback when the active store is set to null', () => {
    mockGetState.mockReturnValue({ key: 'active' })

    const { result } = renderHook(() =>
      useActiveWorkspaceState((s) => (s as unknown as { key: string }).key, 'no-workspace'),
    )

    expect(result.current).toBe('active')

    act(() => {
      capturedChangeListener?.(null)
    })

    expect(result.current).toBe('no-workspace')
  })

  describe('equality guard (default shallow)', () => {
    it('does not notify the consumer when the selector returns a fresh-but-shallow-equal array', () => {
      mockGetState.mockReturnValue({ items: ['a', 'b'] })
      let renderCount = 0

      const { result } = renderHook(() => {
        renderCount++
        return useActiveWorkspaceState((s) => (s as unknown as { items: string[] }).items, [])
      })

      const initialValue = result.current
      const renderCountAfterMount = renderCount

      act(() => {
        // A brand-new array instance with the same contents — this is what
        // every workspace-store write produces for unrelated derived slices.
        capturedStoreSubscriber?.({ items: ['a', 'b'] })
      })

      expect(renderCount).toBe(renderCountAfterMount)
      expect(result.current).toBe(initialValue)
    })

    it('notifies the consumer when the selected value actually changes', () => {
      mockGetState.mockReturnValue({ items: ['a', 'b'] })
      let renderCount = 0

      const { result } = renderHook(() => {
        renderCount++
        return useActiveWorkspaceState((s) => (s as unknown as { items: string[] }).items, [])
      })

      const renderCountAfterMount = renderCount

      act(() => {
        capturedStoreSubscriber?.({ items: ['a', 'c'] })
      })

      expect(renderCount).toBe(renderCountAfterMount + 1)
      expect(result.current).toEqual(['a', 'c'])
    })

    it('respects a custom equalityFn passed by the caller', () => {
      const alwaysEqual = vi.fn(() => true)
      mockGetState.mockReturnValue({ key: 'a' })

      const { result } = renderHook(() =>
        useActiveWorkspaceState(
          (s) => (s as unknown as { key: string }).key,
          'fallback',
          alwaysEqual,
        ),
      )

      expect(result.current).toBe('a')

      act(() => {
        // Custom equalityFn always reports "equal" — even a real change to
        // the underlying value must not notify the consumer.
        capturedStoreSubscriber?.({ key: 'b' })
      })

      expect(result.current).toBe('a')
      expect(alwaysEqual).toHaveBeenCalledWith('a', 'b')
    })

    it('resets the equality baseline when the active workspace store instance changes', () => {
      const firstItems = ['a', 'b']
      mockGetState.mockReturnValue({ items: firstItems })

      const { result } = renderHook(() =>
        useActiveWorkspaceState((s) => (s as unknown as { items: string[] }).items, []),
      )

      expect(result.current).toBe(firstItems)

      // Switch to a different store whose initial value is shallow-equal in
      // *content* to the old value but a distinct array instance — this is a
      // genuine workspace switch, not a same-store write.
      const secondItems = ['a', 'b']
      const nextStore = {
        getState: vi.fn(() => ({ items: secondItems })),
        subscribe: vi.fn((fn: (state: unknown) => void) => {
          capturedStoreSubscriber = fn
          return () => {}
        }),
      }

      act(() => {
        capturedChangeListener?.(nextStore)
      })

      // The switch always applies, regardless of shallow equality with the
      // previous value.
      expect(result.current).toBe(secondItems)

      // A subsequent write on the NEW store with a shallow-equal array must
      // be compared against the NEW baseline (secondItems) and skipped —
      // proving the baseline was reset to the new store, not left stale.
      const thirdItems = ['a', 'b']
      act(() => {
        capturedStoreSubscriber?.({ items: thirdItems })
      })

      expect(result.current).toBe(secondItems)
    })
  })
})
