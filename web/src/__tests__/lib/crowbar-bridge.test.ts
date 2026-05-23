import { describe, it, expect, vi } from 'vitest'
import {
  terminalWrite,
  terminalResize,
  terminalClose,
  terminalListen,
  clipboardSet,
  clipboardPaste,
  clipboardGet,
  clipboardClear,
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

  it('clipboardSet stores entries in memory', async () => {
    await clipboardSet([{ path: '/foo', is_dir: false }], 'copy')
    const state = await clipboardGet()
    expect(state).toEqual({ entries: [{ path: '/foo', is_dir: false }], operation: 'copy' })
  })

  it('clipboardPaste returns empty array', async () => {
    const result = await clipboardPaste('/target')
    expect(result).toEqual([])
  })

  it('clipboardClear nulls the clipboard', async () => {
    await clipboardSet([{ path: '/foo', is_dir: false }], 'copy')
    await clipboardClear()
    const state = await clipboardGet()
    expect(state).toBeNull()
  })

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
