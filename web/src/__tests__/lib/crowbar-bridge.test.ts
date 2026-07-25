import { describe, it, expect, vi } from 'vitest'
import {
  terminalWrite,
  terminalResize,
  terminalClose,
  terminalListen,
  setWindowTransparency,
  setMacOSWindowAppearance,
  toggleMenuBar,
} from '@/lib/crowbar-bridge'

describe('crowbar-bridge', () => {
  it('terminalWrite resolves without error', async () => {
    await expect(terminalWrite('id-1', 'hello')).resolves.toBeUndefined()
  })

  it('terminalResize resolves without error', async () => {
    await expect(terminalResize('id-1', 24, 80)).resolves.toBeUndefined()
  })

  it('terminalClose resolves without error', async () => {
    await expect(terminalClose('id-1')).resolves.toBeUndefined()
  })

  it('terminalListen returns an unlisten function', () => {
    const unlisten = terminalListen('id-1', vi.fn())
    expect(typeof unlisten).toBe('function')
    expect(() => unlisten()).not.toThrow()
  })

  // The bridge's in-memory file clipboard moved to the file-explorer clipboard
  // store when paste stopped being a `return []` stub — its behaviour is covered
  // by features/file-explorer/file-explorer-clipboard-store.test.ts.

  it('setWindowTransparency resolves without error', async () => {
    await expect(setWindowTransparency(true)).resolves.toBeUndefined()
  })

  it('setMacOSWindowAppearance resolves without error', async () => {
    await expect(setMacOSWindowAppearance('dark', false)).resolves.toBeUndefined()
  })

  it('toggleMenuBar resolves without error', async () => {
    await expect(toggleMenuBar(true)).resolves.toBeUndefined()
  })
})
