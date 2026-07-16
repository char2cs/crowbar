import { describe, it, expect, beforeEach } from 'vitest'
import {
  retain,
  release,
  __resetAttachRefcountForTest,
} from '@/features/terminal/lib/attach-refcount'

const CONN = 'conn-1'

describe('attach-refcount', () => {
  beforeEach(() => {
    // Process-global ledger — reset so counts don't bleed between tests.
    __resetAttachRefcountForTest()
  })

  it('two views on one connection: the first release keeps it, the last release frees it', () => {
    // Two mounted attach-only views briefly share ONE connectionId during a pane
    // move. The outgoing view releasing must NOT detach the transport the incoming
    // view still depends on.
    retain(CONN)
    retain(CONN)

    expect(release(CONN)).toBe(false) // a co-view still holds it → do NOT detach
    expect(release(CONN)).toBe(true) // last holder → caller should detach
  })

  it('a single retain/release detaches on the one release', () => {
    retain(CONN)
    expect(release(CONN)).toBe(true)
  })

  it('releasing an unknown connection returns true (safe default: detach)', () => {
    // Nothing ever retained it (or it was already fully released). A caller that
    // believes it owns a transport should be free to detach — this also preserves
    // the pre-refcount behavior for any connection the ledger never saw.
    expect(release('never-retained')).toBe(true)
  })

  it('a fully-released connection is forgotten: the next release is again the safe default', () => {
    retain(CONN)
    expect(release(CONN)).toBe(true) // count hits 0, entry dropped
    expect(release(CONN)).toBe(true) // now unknown again → true
  })

  it('counts are per-connection and independent', () => {
    retain('a')
    retain('a')
    retain('b')

    expect(release('b')).toBe(true) // b had a single holder
    expect(release('a')).toBe(false) // a still has one holder left
    expect(release('a')).toBe(true) // now a's last holder
  })
})
