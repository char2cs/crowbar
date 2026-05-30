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
})
