import { describe, expect, it } from 'vitest'
import { decodeFrame, FrameError } from '@/core/stream/frames'

// Copied from api/internal/domain/stream/testdata/frames.json, the daemon's golden file.
const golden = [
  { t: 'ready', epoch: 'e1' },
  { t: 'entity', scope: 'projects', kind: 'operation', id: 'op1', v: 7, data: { id: 'op1' } },
  { t: 'entity', scope: 'projects', kind: 'operation', id: 'op1', v: 8, gone: true },
  { t: 'synced', scope: 'projects' },
  {
    t: 'closed',
    scope: 'nope:x',
    reason: 'refused',
    problem: { code: 'invalid_argument', message: 'stream: unknown scope kind "nope"' },
  },
  { t: 'closed', scope: 'projects', reason: 'resync' },
  { t: 'error', problem: { code: 'invalid_argument', message: 'stream: unknown op "gossip"' } },
  { t: 'signal', scope: 'git:w1', kind: 'git_status', data: { branch: 'main' } },
  { t: 'entity', scope: 'ws:w1', kind: 'review_thread', id: 'th1', v: 9, data: { id: 'th1' } },
  {
    t: 'row',
    scope: 'chat:c1',
    row: {
      chatId: 'c1',
      seq: 3,
      kind: 'user_message',
      turnId: 't1',
      at: '2026-10-31T12:00:00Z',
      body: { text: 'hi' },
    },
  },
  { t: 'synced', scope: 'chat:c1', seq: 3 },
  {
    t: 'delta',
    scope: 'chat:c1',
    delta: { id: 'm1', turnId: 't1', stream: 'answer', n: 1, text: 'he' },
  },
  { t: 'closed', scope: 'chat:c1', reason: 'resync', seq: 2 },
  { t: 'closed', scope: 'chat:c1', reason: 'deleted' },
  { t: 'entity', scope: 'projects', kind: 'presence', id: 'c1', v: 10, data: { viewers: 2 } },
  { t: 'entity', scope: 'projects', kind: 'presence', id: 'c1', v: 11, gone: true },
]

describe('decodeFrame', () => {
  it('decodes every frame type', () => {
    for (const raw of golden) expect(decodeFrame(raw)).toEqual(raw)
  })

  it('rejects an unknown t', () => {
    expect(() => decodeFrame({ t: 'gossip', scope: 'projects' })).toThrow(FrameError)
    expect(() => decodeFrame('not a frame')).toThrow(FrameError)
  })

  it('rejects an unknown field', () => {
    expect(() => decodeFrame({ t: 'synced', scope: 'projects', extra: 1 })).toThrow(FrameError)
    expect(() =>
      decodeFrame({ t: 'entity', scope: 'projects', kind: 'operation', id: 'op1', v: 'one' }),
    ).toThrow(FrameError)
    expect(() =>
      decodeFrame({ t: 'entity', scope: 'projects', kind: 'stranger', id: 'x', v: 1 }),
    ).toThrow(FrameError)
  })

  it('rejects a row frame without its row', () => {
    expect(() => decodeFrame({ t: 'row', scope: 'chat:c1' })).toThrow(FrameError)
    expect(() => decodeFrame({ t: 'delta', scope: 'chat:c1', delta: 'he' })).toThrow(FrameError)
  })

  it('rejects a signal without its kind', () => {
    expect(() => decodeFrame({ t: 'signal', scope: 'git:w1', data: {} })).toThrow(FrameError)
  })

  it('names the scope of a frame it cannot decode', () => {
    try {
      decodeFrame({ t: 'synced', scope: 'projects', extra: 1 })
      expect.unreachable()
    } catch (err) {
      expect((err as FrameError).scope).toBe('projects')
    }
  })
})
