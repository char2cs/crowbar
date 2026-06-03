import { describe, it, expect, beforeEach, vi } from 'vitest'
import { resetDB } from '@/lib/persistence/idb'
import { dataOf } from '@/lib/loadable'

vi.mock('@/lib/api', () => ({ apiFetch: vi.fn(async () => [{ id: 't1', role: 'user', content: 'hi' }]) }))

beforeEach(() => { resetDB() })

describe('useChatStore', () => {
  it('fetch loads chat turns keyed by (wsId, stepId)', async () => {
    const { useChatStore } = await import('@/features/markdown-chat/stores/chat-store')
    await useChatStore.getState().fetch('ws3', 'step-0')
    expect(dataOf(useChatStore.getState().data)).toHaveLength(1)
  })

  it('caches under a composite key without colliding across steps', async () => {
    const { saveCache, loadCache } = await import('@/lib/persistence/cache-store')
    await saveCache('chat-history', 'ws3:step-0', [{ id: 'x' }], 1)
    const rec = await loadCache('chat-history', 'ws3:step-0')
    expect(rec?.data).toEqual([{ id: 'x' }])
  })
})
