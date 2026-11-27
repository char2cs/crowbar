import { act } from '@testing-library/react'
import { IDBFactory } from 'fake-indexeddb'
import { getDB } from '@/lib/persistence/idb'

export async function cacheReadsSettled() {
  await act(async () => {
    await (await getDB()).get('chat-feeds', ['~', '~'])
  })
}

export function emptyFeedCache() {
  globalThis.indexedDB = new IDBFactory()
}
