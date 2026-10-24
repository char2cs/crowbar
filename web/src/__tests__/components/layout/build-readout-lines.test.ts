import { describe, it, expect } from 'vitest'
import {
  formatReadout,
  netStateOf,
  type ReadoutSnapshot,
} from '@/components/layout/build-readout-lines'

const snapshot = (over: Partial<ReadoutSnapshot> = {}): ReadoutSnapshot => ({
  info: { channel: 'dev', timestamp: new Date(2026, 9, 24, 16, 47).toISOString() },
  net: 'ok',
  reconnects: 0,
  frame: { fps: 129, maxDt: 18, drops: 2 },
  counts: { pty: 4, ag: 2, ws: 3 },
  ...over,
})

const line1 = (s: ReadoutSnapshot) => {
  const l = formatReadout(s)
  return [l.build, l.net, l.reconnects].filter(Boolean).join('  ')
}
const line2 = (s: ReadoutSnapshot) => formatReadout(s).stats

describe('netStateOf', () => {
  it('maps the connection owner onto the readout states', () => {
    expect(netStateOf('connected', false)).toBe('ok')
    expect(netStateOf('connected', true)).toBe('reconnecting')
    expect(netStateOf('disconnected', false)).toBe('down')
    expect(netStateOf('idle', false)).toBe('idle')
  })
})

describe('formatReadout', () => {
  it('reads the reference two lines', () => {
    const s = snapshot({ reconnects: 2 })
    expect(line1(s)).toBe('dev 10-24 16:47  net ok  rc2')
    expect(line2(s)).toBe('129fps 18ms/2  pty4 ag2 ws3')
    expect(formatReadout(s).netWarn).toBe(false)
  })

  it('shows the version for beta and only the version for release', () => {
    const beta = snapshot({ info: { channel: 'beta', version: '0.0.0-beta.1' } })
    expect(formatReadout(beta).build).toBe('beta 0.0.0-beta.1')
    const release = snapshot({ info: { channel: 'release', version: '0.0.0' } })
    expect(formatReadout(release).build).toBe('0.0.0')
  })

  it('leaves rc off until the first reconnect', () => {
    expect(line1(snapshot())).toBe('dev 10-24 16:47  net ok')
    expect(formatReadout(snapshot()).reconnects).toBe('')
  })

  it('hides the drops when there are none', () => {
    expect(line2(snapshot({ frame: { fps: 120, maxDt: 9, drops: 0 } }))).toBe(
      '120fps 9ms  pty4 ag2 ws3',
    )
  })

  it('shows dashes before the first frame window', () => {
    expect(line2(snapshot({ frame: { fps: 0, maxDt: 0, drops: 0 } }))).toBe(
      '--fps --ms  pty4 ag2 ws3',
    )
  })

  it('warns while reconnecting, keeping the counts', () => {
    const l = formatReadout(snapshot({ net: 'reconnecting' }))
    expect(l.net).toBe('net reconnecting')
    expect(l.netWarn).toBe(true)
    expect(l.stats).toBe('129fps 18ms/2  pty4 ag2 ws3')
  })

  it('when DOWN, warns and drops the counts, which are stale', () => {
    const l = formatReadout(snapshot({ net: 'down', reconnects: 1 }))
    expect(l.net).toBe('net DOWN')
    expect(l.netWarn).toBe(true)
    expect(l.stats).toBe('129fps 18ms/2')
    expect(l.reconnects).toBe('rc1')
  })

  it('says nothing about a backend no channel has reported on yet', () => {
    const l = formatReadout(snapshot({ net: 'idle' }))
    expect(l.net).toBe('net --')
    expect(l.netWarn).toBe(false)
  })

  it('keeps both lines within 40 characters at the widest states', () => {
    const wide = snapshot({
      net: 'reconnecting',
      reconnects: 99,
      frame: { fps: 240, maxDt: 499, drops: 99 },
      counts: { pty: 99, ag: 99, ws: 99 },
    })
    expect(line1(wide).length).toBeLessThanOrEqual(40)
    expect(line2(wide).length).toBeLessThanOrEqual(40)
  })
})
