import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest'
import { exportDiagnostics } from '@/lib/crowbar-bridge'

const platform = vi.hoisted(() => ({ mobile: false }))
vi.mock('@/utils/platform', async (importOriginal) => ({
  ...(await importOriginal<typeof import('@/utils/platform')>()),
  get IS_MOBILE() {
    return platform.mobile
  },
}))

type TauriWindow = Window & { __TAURI_INTERNALS__?: { invoke: (cmd: string) => Promise<unknown> } }
const tauriWindow = window as TauriWindow

describe('exportDiagnostics', () => {
  beforeEach(() => {
    delete tauriWindow.__TAURI_INTERNALS__
  })
  afterEach(() => {
    platform.mobile = false
    delete tauriWindow.__TAURI_INTERNALS__
  })

  // The phone app is Tauri, but the bundle (daemon logs, dumps) is a desktop-host command.
  it('rejects in the phone app without invoking', async () => {
    platform.mobile = true
    const invoke = vi.fn()
    tauriWindow.__TAURI_INTERNALS__ = { invoke }
    await expect(exportDiagnostics()).rejects.toThrow(/desktop app/)
    expect(invoke).not.toHaveBeenCalled()
  })

  it('invokes the diagnostics_export command and resolves the bundle path', async () => {
    const invoke = vi.fn().mockResolvedValue('/Users/me/Downloads/crowbar-diagnostics-1.zip')
    tauriWindow.__TAURI_INTERNALS__ = { invoke }

    await expect(exportDiagnostics()).resolves.toBe('/Users/me/Downloads/crowbar-diagnostics-1.zip')
    expect(invoke).toHaveBeenCalledWith('diagnostics_export', undefined)
  })

  it('rejects outside the desktop app instead of invoking nothing', async () => {
    await expect(exportDiagnostics()).rejects.toThrow(/desktop app/)
  })

  it('propagates command failures so the caller can toast them', async () => {
    tauriWindow.__TAURI_INTERNALS__ = {
      invoke: vi.fn().mockRejectedValue(new Error('daemon socket unknown')),
    }

    await expect(exportDiagnostics()).rejects.toThrow('daemon socket unknown')
  })
})
