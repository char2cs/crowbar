import { beforeEach, describe, expect, it, vi } from 'vitest'

const apiFetch = vi.fn()
vi.mock('@/lib/api/fetch', () => ({ apiFetch: (...a: unknown[]) => apiFetch(...a) }))

import { readOlderRows, readRow } from '@/features/agent/api/chat-rows-reads'
import { PAGE_BYTES } from '@/features/agent/lib/feed-window'
import { qualify } from '@/lib/host-path'

beforeEach(() => apiFetch.mockReset())

describe('chat rows reads', () => {
  it('asks the chat’s own host for the rows before a seq, by wire size', async () => {
    apiFetch.mockResolvedValue({ rows: [], from: 0, more: false })
    await readOlderRows('host-2', 'chat/1', 40)
    expect(apiFetch).toHaveBeenCalledWith(
      `${qualify('host-2', '/v0/chats/chat%2F1')}/rows?before=40&bytes=${PAGE_BYTES}`,
    )
  })

  it('reads one row whole', async () => {
    const row = { seq: 7 }
    apiFetch.mockResolvedValue(row)
    await expect(readRow('local', 'c1', 7)).resolves.toBe(row)
    expect(apiFetch).toHaveBeenCalledWith(`${qualify('local', '/v0/chats/c1')}/rows/7`)
  })
})
