import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest'
import { exportDiagnostics } from '@/features/settings/lib/diagnostics-export'

type TauriWindow = Window & { __TAURI_INTERNALS__?: { invoke: (cmd: string) => Promise<unknown> } }
const tauriWindow = window as TauriWindow

describe('exportDiagnostics', () => {
  beforeEach(() => {
    delete tauriWindow.__TAURI_INTERNALS__
  })
  afterEach(() => {
    delete tauriWindow.__TAURI_INTERNALS__
  })

  it('invokes the diagnostics_export command and resolves the bundle path', async () => {
    const invoke = vi.fn().mockResolvedValue('/Users/me/Downloads/crowbar-diagnostics-1.zip')
    tauriWindow.__TAURI_INTERNALS__ = { invoke }

    await expect(exportDiagnostics()).resolves.toBe('/Users/me/Downloads/crowbar-diagnostics-1.zip')
    expect(invoke).toHaveBeenCalledWith('diagnostics_export')
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
