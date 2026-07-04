import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'

// openTauriSocket constructs `new Channel<string>()`; a minimal stand-in suffices.
vi.mock('@tauri-apps/api/core', () => ({
  Channel: class {
    onmessage: ((raw: string) => void) | null = null
  },
}))

// openTauriSocket subscribes to `terminal:transport-dropped` via the dynamically
// imported event module; listen resolves to this unlisten spy so the M6 cleanup
// (unlisten on terminal_open failure) is observable.
const unlistenSpy = vi.fn()
vi.mock('@tauri-apps/api/event', () => ({
  listen: vi.fn(async () => unlistenSpy),
}))

import { terminalAttach, terminalHasTransport, __getBridgeInternals } from '@/lib/crowbar-bridge'

describe('crowbar-bridge Tauri terminal_open failure (M6)', () => {
  let invoke: ReturnType<typeof vi.fn>

  beforeEach(() => {
    unlistenSpy.mockClear()
    invoke = vi.fn()
    // isTauri() checks '__TAURI_INTERNALS__' in window; tauriInvoke calls its invoke.
    ;(window as unknown as { __TAURI_INTERNALS__: unknown }).__TAURI_INTERNALS__ = { invoke }
  })

  afterEach(() => {
    delete (window as unknown as { __TAURI_INTERNALS__?: unknown }).__TAURI_INTERNALS__
    __getBridgeInternals().tauriTerminals.clear()
    vi.restoreAllMocks()
  })

  it('removes the phantom entry and unlistens when terminal_open rejects', async () => {
    invoke.mockRejectedValueOnce(new Error('terminal_open failed'))

    await expect(terminalAttach('conn-open-fail', '/base')).rejects.toThrow('terminal_open failed')

    // No lingering transport: terminalHasTransport must report false, and the
    // internal map entry + its transport-dropped listener must both be gone.
    expect(terminalHasTransport('conn-open-fail')).toBe(false)
    expect(__getBridgeInternals().tauriTerminals.has('conn-open-fail')).toBe(false)
    expect(unlistenSpy).toHaveBeenCalledTimes(1)
  })

  it('keeps the entry when terminal_open succeeds (control)', async () => {
    invoke.mockResolvedValueOnce(undefined)

    await terminalAttach('conn-ok', '/base')

    expect(terminalHasTransport('conn-ok')).toBe(true)
    expect(unlistenSpy).not.toHaveBeenCalled()
  })
})
