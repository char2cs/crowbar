import { describe, it, expect } from 'vitest'
import {
  setWindowTransparency,
  setMacOSWindowAppearance,
  setTrafficLightPosition,
} from '@/lib/crowbar-bridge'

// The terminal transport is covered by crowbar-bridge-terminal.test.ts.
describe('crowbar-bridge', () => {
  it('setWindowTransparency resolves without error', async () => {
    await expect(setWindowTransparency(true)).resolves.toBeUndefined()
  })

  it('setMacOSWindowAppearance resolves without error', async () => {
    await expect(setMacOSWindowAppearance('dark', false)).resolves.toBeUndefined()
  })

  it('setTrafficLightPosition resolves without error', async () => {
    await expect(setTrafficLightPosition(12, 33)).resolves.toBeUndefined()
  })
})
