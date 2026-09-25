import { describe, it, expect } from 'vitest'
import {
  setWindowTransparency,
  setMacOSWindowAppearance,
  setTrafficLightPosition,
  toggleMenuBar,
} from '@/lib/crowbar-bridge'

// The terminal transport is covered by crowbar-bridge-terminal.test.ts.
describe('crowbar-bridge', () => {
  // The bridge's in-memory file clipboard moved to the file-explorer clipboard
  // store when paste stopped being a `return []` stub — its behaviour is covered
  // by features/file-explorer/file-explorer-clipboard-store.test.ts.

  it('setWindowTransparency resolves without error', async () => {
    await expect(setWindowTransparency(true)).resolves.toBeUndefined()
  })

  it('setMacOSWindowAppearance resolves without error', async () => {
    await expect(setMacOSWindowAppearance('dark', false)).resolves.toBeUndefined()
  })

  it('setTrafficLightPosition resolves without error', async () => {
    await expect(setTrafficLightPosition(12, 33)).resolves.toBeUndefined()
  })

  it('toggleMenuBar resolves without error', async () => {
    await expect(toggleMenuBar(true)).resolves.toBeUndefined()
  })
})
