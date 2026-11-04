import type { AgentChatMessage } from '@/features/agent/api/agent-api'
import { getDB } from './idb'

/** Chats whose transcript is kept; the least recently written go first. */
const CHAT_HISTORY_MAX_CHATS = 200

export interface ChatHistory {
  /** The newest page of the chat, as the ledger held it. */
  messages: AgentChatMessage[]
  hasOlder: boolean
}

// Keyed by chatId (a uuid, unique across hosts) and tagged with its host so removing a
// host can forget its transcripts. Best-effort like every cache here: a failure is a miss.

export async function saveChatHistory(
  chatId: string,
  history: ChatHistory,
  hostId: string,
  fetchedAt: number = Date.now(),
): Promise<void> {
  try {
    const db = await getDB()
    const tx = db.transaction('chat-history', 'readwrite')
    await tx.store.put({ key: chatId, data: history, fetchedAt, hostId })
    const excess = (await tx.store.count()) - CHAT_HISTORY_MAX_CHATS
    if (excess > 0) {
      let cursor = await tx.store.index('fetchedAt').openKeyCursor()
      for (let evicted = 0; cursor && evicted < excess; evicted++) {
        await tx.store.delete(cursor.primaryKey)
        cursor = await cursor.continue()
      }
    }
    await tx.done
  } catch {
    /* best-effort cache write — ignore IDB failures */
  }
}

export async function loadChatHistory(chatId: string): Promise<ChatHistory | undefined> {
  try {
    const db = await getDB()
    return (await db.get('chat-history', chatId))?.data as ChatHistory | undefined
  } catch {
    return undefined
  }
}

export async function deleteChatHistoryOfHost(hostId: string): Promise<void> {
  try {
    const db = await getDB()
    const tx = db.transaction('chat-history', 'readwrite')
    let cursor = await tx.store.index('hostId').openKeyCursor(hostId)
    while (cursor) {
      await tx.store.delete(cursor.primaryKey)
      cursor = await cursor.continue()
    }
    await tx.done
  } catch {
    /* best-effort cache delete — ignore IDB failures */
  }
}
