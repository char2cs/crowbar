import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { TokenizerWorkerClient } from '@/features/editor/lib/wasm-parser/tokenizer-worker-client'

// Minimal mock Worker that captures posted messages and exposes helpers to
// simulate replies and errors.
function makeWorkerMock() {
  const posted: unknown[] = []
  let onmessage: ((ev: MessageEvent) => void) | null = null
  let onerror: ((ev: ErrorEvent) => void) | null = null

  const worker = {
    posted,
    postMessage: vi.fn((msg: unknown) => {
      posted.push(msg)
    }),
    set onmessage(handler: ((ev: MessageEvent) => void) | null) {
      onmessage = handler
    },
    get onmessage() {
      return onmessage
    },
    set onerror(handler: ((ev: ErrorEvent) => void) | null) {
      onerror = handler
    },
    get onerror() {
      return onerror
    },
    /** Simulate a successful reply from the worker. */
    reply(id: number, extra: Record<string, unknown> = {}) {
      onmessage?.({ data: { id, ok: true, tokens: [], normalizedText: '', ...extra } } as MessageEvent)
    },
    /** Simulate an error reply from the worker. */
    replyError(id: number, error: string) {
      onmessage?.({ data: { id, ok: false, error } } as MessageEvent)
    },
    /** Simulate a fatal worker error (onerror). */
    fatalError(message = 'fatal') {
      onerror?.({ error: new Error(message) } as ErrorEvent)
    },
  }
  return worker
}

describe('TokenizerWorkerClient', () => {
  let workerMock: ReturnType<typeof makeWorkerMock>

  beforeEach(() => {
    workerMock = makeWorkerMock()
    // Replace global Worker so TokenizerWorkerClient uses our mock.
    vi.stubGlobal('Worker', vi.fn(() => workerMock))
    vi.useFakeTimers()
  })

  afterEach(() => {
    vi.useRealTimers()
    vi.unstubAllGlobals()
  })

  // ── (a) Timeout / no-leak ────────────────────────────────────────────────

  it('(a) rejects after timeout and removes entry from pending when worker never replies', async () => {
    const SHORT_TIMEOUT = 200
    const client = new TokenizerWorkerClient(SHORT_TIMEOUT)

    const promise = client.tokenize({
      bufferId: 'buf-1',
      content: 'const x = 1',
      languageId: 'typescript',
      mode: 'full',
    })

    // Advance past the timeout.
    vi.advanceTimersByTime(SHORT_TIMEOUT + 1)

    await expect(promise).rejects.toThrow('tokenizer worker timed out')

    // Verify no pending entry leaked: posting another request for a *different*
    // buffer and immediately resolving it should work cleanly (pending map has
    // no stale entries interfering).
    const promise2 = client.tokenize({
      bufferId: 'buf-2',
      content: 'let y = 2',
      languageId: 'typescript',
      mode: 'full',
    })
    const lastMsg = workerMock.posted[workerMock.posted.length - 1] as { id: number }
    workerMock.reply(lastMsg.id)
    await expect(promise2).resolves.toEqual({ tokens: [], normalizedText: '' })
  })

  // ── (b) Normal reply clears timeout and resolves ─────────────────────────

  it('(b) resolves normally when worker replies and does NOT later reject', async () => {
    const SHORT_TIMEOUT = 500
    const client = new TokenizerWorkerClient(SHORT_TIMEOUT)

    const promise = client.tokenize({
      bufferId: 'buf-a',
      content: 'hello',
      languageId: 'plaintext',
      mode: 'full',
    })

    // Reply immediately (before timeout fires).
    const msg = workerMock.posted[0] as { id: number }
    workerMock.reply(msg.id, { tokens: [{ type: 'keyword', start: 0, end: 5 }], normalizedText: 'hello' })

    const result = await promise
    expect(result).toEqual({
      tokens: [{ type: 'keyword', start: 0, end: 5 }],
      normalizedText: 'hello',
    })

    // Advance well past the timeout — the promise should not reject (it already resolved).
    let lateRejection: unknown = undefined
    promise.catch((err) => {
      lateRejection = err
    })
    vi.advanceTimersByTime(SHORT_TIMEOUT + 1)
    // Allow micro-task queue to drain.
    await Promise.resolve()
    expect(lateRejection).toBeUndefined()
  })

  // ── Part 2: supersede stale requests for the same buffer ─────────────────

  it('rejects a prior in-flight tokenize request when a newer one for the same buffer is posted', async () => {
    const SHORT_TIMEOUT = 1000
    const client = new TokenizerWorkerClient(SHORT_TIMEOUT)

    const first = client.tokenize({
      bufferId: 'shared-buf',
      content: 'v1',
      languageId: 'typescript',
      mode: 'full',
    })

    // Post a second request for the SAME buffer before the first replies.
    const second = client.tokenize({
      bufferId: 'shared-buf',
      content: 'v2',
      languageId: 'typescript',
      mode: 'full',
    })

    // First request should be superseded immediately (synchronously inside post()).
    await expect(first).rejects.toThrow('superseded')

    // Resolve the second request normally.
    const lastMsg = workerMock.posted[workerMock.posted.length - 1] as { id: number }
    workerMock.reply(lastMsg.id, { normalizedText: 'v2' })
    await expect(second).resolves.toEqual({ tokens: [], normalizedText: 'v2' })
  })
})
