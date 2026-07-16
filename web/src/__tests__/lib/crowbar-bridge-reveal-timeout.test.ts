import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'

import { revealItemInFinder } from '@/lib/crowbar-bridge'

describe('crowbar-bridge revealItemInFinder timeout guard (Task 30)', () => {
  let invoke: ReturnType<typeof vi.fn>

  beforeEach(() => {
    invoke = vi.fn()
    // isTauri() checks '__TAURI_INTERNALS__' in window; tauriInvoke calls its invoke.
    ;(window as unknown as { __TAURI_INTERNALS__: unknown }).__TAURI_INTERNALS__ = { invoke }
  })

  afterEach(() => {
    delete (window as unknown as { __TAURI_INTERNALS__?: unknown }).__TAURI_INTERNALS__
    vi.useRealTimers()
    vi.restoreAllMocks()
  })

  it('resolves normally when the invoke settles before the timeout (control)', async () => {
    invoke.mockResolvedValueOnce(undefined)

    await expect(revealItemInFinder('/tmp')).resolves.toBeUndefined()
    expect(invoke).toHaveBeenCalledWith('reveal_in_finder', { path: '/tmp' })
  })

  it('propagates a rejection from the invoke unchanged (control)', async () => {
    invoke.mockRejectedValueOnce(new Error('reveal_in_finder failed'))

    await expect(revealItemInFinder('/tmp')).rejects.toThrow('reveal_in_finder failed')
  })

  // The bug this guards against: an invoke that never settles (hung main
  // thread, denied capability, whatever) previously left the caller's
  // `.catch` — which surfaces an error toast — waiting forever in silence.
  it('rejects with a timeout error when the invoke never settles', async () => {
    vi.useFakeTimers()
    invoke.mockReturnValueOnce(new Promise(() => {})) // never resolves or rejects

    const assertion = expect(revealItemInFinder('/tmp')).rejects.toThrow(
      'reveal_in_finder timed out',
    )
    await vi.advanceTimersByTimeAsync(3_000)
    await assertion
  })

  it('is a no-op outside Tauri regardless of the invoke', async () => {
    delete (window as unknown as { __TAURI_INTERNALS__?: unknown }).__TAURI_INTERNALS__

    await expect(revealItemInFinder('/tmp')).resolves.toBeUndefined()
    expect(invoke).not.toHaveBeenCalled()
  })
})
