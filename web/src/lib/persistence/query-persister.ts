import { createAsyncStoragePersister } from '@tanstack/query-async-storage-persister'
import { getDB } from './idb'

const idbAsyncStorage = {
  getItem: async (key: string): Promise<string | null> => {
    const result = await (await getDB()).get('query-cache', key)
    return result ?? null
  },

  setItem: async (key: string, value: string): Promise<void> => {
    await (await getDB()).put('query-cache', value, key)
  },

  removeItem: async (key: string): Promise<void> => {
    await (await getDB()).delete('query-cache', key)
  },
}

export const persister = createAsyncStoragePersister({
  storage: idbAsyncStorage,
  key: 'crowbar-query-cache',
})
