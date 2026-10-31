import { beforeEach, describe, expect, it } from 'vitest'
import { IDBFactory } from 'fake-indexeddb'
import { resetDB } from '@/lib/persistence/idb'
import {
  deleteChatHistoryOfHost,
  loadChatHistory,
  saveChatHistory,
} from '@/lib/persistence/chat-history'

const CHAT_HISTORY_MAX_CHATS = 200

beforeEach(() => {
  globalThis.indexedDB = new IDBFactory()
  resetDB()
})

const record = (text: string) => ({
  messages: [
    { turnId: 't1', sequence: 1, role: 'user' as const, providerId: '', text, at: '2026-08-24' },
  ],
  hasOlder: true,
})

describe('chat history cache', () => {
  it('reads nothing for a chat that was never saved', async () => {
    expect(await loadChatHistory('c1')).toBeUndefined()
  })

  it('round-trips a transcript and overwrites it on the next save', async () => {
    await saveChatHistory('c1', record('first'), 'h1')
    await saveChatHistory('c1', record('second'), 'h1')

    expect((await loadChatHistory('c1'))?.messages[0].text).toBe('second')
    expect((await loadChatHistory('c1'))?.hasOlder).toBe(true)
  })

  it('keeps only the most recently written chats once past the cap', async () => {
    for (let i = 0; i <= CHAT_HISTORY_MAX_CHATS; i++) {
      await saveChatHistory(`c${i}`, record('x'), 'h1', i)
    }

    expect(await loadChatHistory('c0')).toBeUndefined()
    expect(await loadChatHistory('c1')).toBeDefined()
    expect(await loadChatHistory(`c${CHAT_HISTORY_MAX_CHATS}`)).toBeDefined()
  })

  it('treats a chat as recent again when it is rewritten', async () => {
    for (let i = 0; i < CHAT_HISTORY_MAX_CHATS; i++)
      await saveChatHistory(`c${i}`, record('x'), 'h1', i)
    await saveChatHistory('c0', record('again'), 'h1', CHAT_HISTORY_MAX_CHATS)

    await saveChatHistory('new', record('x'), 'h1', CHAT_HISTORY_MAX_CHATS + 1)

    expect(await loadChatHistory('c0')).toBeDefined()
    expect(await loadChatHistory('c1')).toBeUndefined()
  })

  it('forgets every transcript of a removed host and keeps the other hosts', async () => {
    await saveChatHistory('a1', record('x'), 'h1')
    await saveChatHistory('a2', record('x'), 'h1')
    await saveChatHistory('b1', record('x'), 'h2')

    await deleteChatHistoryOfHost('h1')

    expect(await loadChatHistory('a1')).toBeUndefined()
    expect(await loadChatHistory('a2')).toBeUndefined()
    expect(await loadChatHistory('b1')).toBeDefined()
  })
})
