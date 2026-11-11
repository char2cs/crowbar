import { afterEach, describe, expect, it, vi } from 'vitest'
import { progressFetch } from '@/lib/api/xhr-transport'

class FakeXhr {
  static last: FakeXhr
  upload: {
    onprogress?: (e: { lengthComputable: boolean; loaded: number; total: number }) => void
  } = {}
  onload?: () => void
  onerror?: () => void
  onabort?: () => void
  status = 201
  statusText = 'Created'
  response = new TextEncoder().encode('{"ok":true}').buffer
  open = vi.fn()
  setRequestHeader = vi.fn()
  getAllResponseHeaders = () => 'content-type: application/json\r\n'
  send = vi.fn()
  abort = () => this.onabort?.()
  constructor() {
    FakeXhr.last = this
  }
}

afterEach(() => vi.unstubAllGlobals())

describe('progressFetch', () => {
  it('reports what has gone and answers like fetch', async () => {
    vi.stubGlobal('XMLHttpRequest', FakeXhr)
    const seen: Array<[number, number]> = []

    const pending = progressFetch((loaded, total) => seen.push([loaded, total]))('/up', {
      method: 'POST',
      headers: { 'Content-Type': 'text/plain' },
      body: new Uint8Array([1, 2]),
    })
    FakeXhr.last.upload.onprogress?.({ lengthComputable: true, loaded: 1, total: 2 })
    FakeXhr.last.onload?.()

    const response = await pending
    expect(seen).toEqual([[1, 2]])
    expect(response.status).toBe(201)
    expect(await response.json()).toEqual({ ok: true })
    expect(FakeXhr.last.setRequestHeader).toHaveBeenCalledWith('content-type', 'text/plain')
  })

  it('rejects as fetch does: a TypeError for no answer, an AbortError for an abort', async () => {
    vi.stubGlobal('XMLHttpRequest', FakeXhr)
    const controller = new AbortController()
    const send = progressFetch(() => {})

    const dropped = send('/up', { method: 'POST' })
    FakeXhr.last.onerror?.()
    await expect(dropped).rejects.toBeInstanceOf(TypeError)

    const cancelled = send('/up', { method: 'POST', signal: controller.signal })
    controller.abort()
    await expect(cancelled).rejects.toMatchObject({ name: 'AbortError' })
  })
})
