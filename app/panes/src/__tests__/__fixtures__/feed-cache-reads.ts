import { act } from '@testing-library/react'
import { IDBFactory } from 'fake-indexeddb'
import { getDB } from '@/lib/persistence/idb'

/** A fresh feed holds its scope once its cache read settles; a read queued behind it settles after. */
export async function cacheReadsSettled() {
  await act(async () => {
    await (await getDB()).get('chat-feeds', ['~', '~'])
  })
}

/** A chat's stored feed is shown at its next attach: a test starts from an empty cache. */
export function emptyFeedCache() {
  globalThis.indexedDB = new IDBFactory()
}
