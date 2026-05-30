import { saveUIPreferences, loadUIPreferences } from '@/lib/persistence/ui-preferences'
import type { UIPreferences } from '@/lib/persistence/schemas'
import { IDBFactory } from 'fake-indexeddb'
import { resetDB } from '@/lib/persistence/idb'

const mockPrefs: UIPreferences = {
  theme: 'github-dark',
  fontSize: 15,
  fontFamily: 'JetBrains Mono Variable',
  tabSize: 2,
  wordWrap: true,
  minimap: false,
  updatedAt: 0,
}

beforeEach(() => {
  resetDB()
  // @ts-expect-error
  globalThis.indexedDB = new IDBFactory()
})

describe('ui preferences persistence', () => {
  it('saves and loads preferences round-trip', async () => {
    await saveUIPreferences(mockPrefs)
    const loaded = await loadUIPreferences()
    expect(loaded?.theme).toBe('github-dark')
    expect(loaded?.fontSize).toBe(15)
    expect(loaded?.minimap).toBe(false)
  })

  it('returns null when no preferences saved', async () => {
    const loaded = await loadUIPreferences()
    expect(loaded).toBeNull()
  })
})
