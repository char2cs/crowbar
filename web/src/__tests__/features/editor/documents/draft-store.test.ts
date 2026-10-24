import { afterEach, beforeAll, beforeEach, describe, expect, it, vi } from 'vitest'
import { MONACO_LOAD_MS } from '@/__tests__/__fixtures__/documents'
import { documentUri } from '@/features/editor/documents/document-ref'
import { clearDraft, loadDrafts } from '@/features/editor/documents/draft-store'
import { loadEditorHost, type EditorHost } from '@/features/editor/host/editor-host'
import { QUIET_PERIOD_MS } from '@/features/editor/lib/buffer-save'
import { workspaceRef } from '@/lib/api/documents'

const DOC = workspaceRef('ws-drafts', 'notes.md')
const URI = documentUri(DOC)

let host: EditorHost

beforeAll(async () => {
  host = await loadEditorHost()
}, MONACO_LOAD_MS)

beforeEach(() => {
  // IndexedDB schedules its own work on setImmediate, which stays real.
  vi.useFakeTimers({ toFake: ['setTimeout', 'clearTimeout', 'Date'] })
  host.documents.open(DOC, 'markdown', 'disk')
})

afterEach(async () => {
  clearDraft(URI)
  host.documents.release(URI)
  vi.useRealTimers()
  await loadDrafts()
})

describe('draft store', () => {
  it('a dirty document is written once after the quiet period, not per keystroke', async () => {
    const puts = vi.spyOn(IDBObjectStore.prototype, 'put')
    for (let i = 1; i <= 30; i++) {
      host.documents.replace(URI, `typed ${i}`)
      vi.advanceTimersByTime(30)
    }

    expect(await loadDrafts()).toEqual(new Map())
    vi.advanceTimersByTime(QUIET_PERIOD_MS)

    const drafts = await loadDrafts()
    expect(drafts.get(URI)).toMatchObject({ uri: URI, text: 'typed 30', savedText: 'disk' })
    expect(puts).toHaveBeenCalledTimes(1)
    puts.mockRestore()
  })

  it('pagehide flushes pending drafts', async () => {
    host.documents.replace(URI, 'typed before leaving')

    window.dispatchEvent(new Event('pagehide'))

    expect((await loadDrafts()).get(URI)?.text).toBe('typed before leaving')
  })

  it('clearing a draft removes it', async () => {
    host.documents.replace(URI, 'typed')
    vi.advanceTimersByTime(QUIET_PERIOD_MS)
    expect((await loadDrafts()).has(URI)).toBe(true)

    clearDraft(URI)

    expect((await loadDrafts()).has(URI)).toBe(false)
  })
})
