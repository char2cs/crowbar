import { describe, expect, it } from 'vitest'
import { formatLogTime, jsonLines, parseLogFrame } from '@/features/console/lib/frames'

const LOG = {
  type: 'log',
  seq: 412,
  time: '2026-10-04T14:02:14.390123Z',
  level: 'warn',
  component: 'release',
  msg: 'channel lookup slow',
  fields: {
    ns: 'github.com/char2cs/crowbar',
    took: '1.8s',
    retry: 1,
    ok: false,
    err: 'boom',
    n: null,
  },
}

describe('parseLogFrame', () => {
  it('decodes a log record and classifies its fields', () => {
    const frame = parseLogFrame(JSON.stringify(LOG))
    if (frame.type !== 'log') throw new Error('expected a log frame')
    expect(frame.record).toMatchObject({ seq: 412, level: 'warn', component: 'release' })
    const kinds = Object.fromEntries(frame.record.fields.map((f) => [f.key, f.kind]))
    expect(kinds).toEqual({
      ns: 'path',
      took: 'duration',
      retry: 'number',
      ok: 'bool',
      err: 'error',
      n: 'text',
    })
  })

  it('decodes ready, with and without reset', () => {
    expect(parseLogFrame('{"type":"ready","seq":3}')).toEqual({
      type: 'ready',
      seq: 3,
      reset: false,
    })
    expect(parseLogFrame('{"type":"ready","seq":3,"reset":true}')).toEqual({
      type: 'ready',
      seq: 3,
      reset: true,
    })
  })

  it('maps unknown levels to info and aliases to their level', () => {
    const level = (l: string) => {
      const f = parseLogFrame({ ...LOG, level: l })
      return f.type === 'log' ? f.record.level : null
    }
    expect(level('weird')).toBe('info')
    expect(level('FATAL')).toBe('error')
    expect(level('trace')).toBe('debug')
  })

  it('turns anything it cannot decode into raw text instead of throwing', () => {
    expect(parseLogFrame('not json')).toEqual({ type: 'raw', text: 'not json' })
    expect(parseLogFrame('[1]')).toEqual({ type: 'raw', text: '[1]' })
    expect(parseLogFrame('{"type":"log","msg":"no seq"}').type).toBe('raw')
    expect(parseLogFrame('{"type":"ready"}').type).toBe('raw')
    expect(parseLogFrame('{"type":"gap"}').type).toBe('raw')
  })
})

describe('formatLogTime and jsonLines', () => {
  it('formats an ISO time as HH:mm:ss.SSS and passes non-dates through', () => {
    expect(formatLogTime('2026-10-04T14:02:14.390123Z')).toMatch(/^\d{2}:\d{2}:\d{2}\.390$/)
    expect(formatLogTime('yesterday')).toBe('yesterday')
  })

  it('quotes strings but not numbers or bools in the record JSON', () => {
    const frame = parseLogFrame(LOG)
    if (frame.type !== 'log') throw new Error('expected a log frame')
    const byKey = Object.fromEntries(jsonLines(frame.record).map((l) => [l.key, l.value]))
    expect(byKey.msg).toBe('"channel lookup slow"')
    expect(byKey.retry).toBe('1')
    expect(byKey.ok).toBe('false')
    expect(byKey.component).toBe('"release"')
  })
})
