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

  it('names the scope of a frame it cannot decode', () => {
    try {
      decodeFrame({ t: 'synced', scope: 'projects', extra: 1 })
      expect.unreachable()
    } catch (err) {
      expect((err as FrameError).scope).toBe('projects')
    }
  })
})
