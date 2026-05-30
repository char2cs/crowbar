import { getDB } from './idb'
import type { UIPreferences } from './schemas'

export async function saveUIPreferences(prefs: UIPreferences): Promise<void> {
  const db = await getDB()
  await db.put('ui-preferences', { ...prefs, updatedAt: Date.now() }, 'global')
}

export async function loadUIPreferences(): Promise<UIPreferences | null> {
  const db = await getDB()
  return (await db.get('ui-preferences', 'global')) ?? null
}
