import { beforeEach, describe, expect, it } from 'vitest'
import type { LogFrame } from '@/features/console/lib/frames'
import { useConsoleStore } from '@/features/console/stores/console-store'

function log(seq: number, msg = `m${seq}`): LogFrame {
  return {
    type: 'log',
    record: { seq, iso: '2026-10-04T14:02:14Z', level: 'info', component: '', msg, fields: [] },
  }
}
const ready = (seq: number, reset = false): LogFrame => ({ type: 'ready', seq, reset })
const BUFFER_CAP = 5000
const messages = () =>
  useConsoleStore.getState().entries.map((e) => (e.kind === 'log' ? e.record.msg : e.kind))

beforeEach(() => {
  useConsoleStore.setState(useConsoleStore.getInitialState())
})

describe('console store ingest', () => {
  it('advances the cursor to the highest seq shown', () => {
    useConsoleStore.getState().ingest([log(1), log(2), ready(2)])
    expect(useConsoleStore.getState().cursor).toBe(2)
    expect(messages()).toEqual(['m1', 'm2'])
  })

  it('drops a record at or below the cursor, so a replay racing live shows each line once', () => {
    const { ingest } = useConsoleStore.getState()
    ingest([log(1), log(2)])
    ingest([log(2), log(3)])
    expect(messages()).toEqual(['m1', 'm2', 'm3'])
  })

  it('on reset discards earlier log lines, shows the restart note and re-reads from the replay', () => {
    const { ingest } = useConsoleStore.getState()
    ingest([log(40), log(41)])
    ingest([log(1), log(2), ready(2, true)])
    expect(messages()).toEqual(['restarted', 'm1', 'm2'])
    expect(useConsoleStore.getState().cursor).toBe(2)
  })

  it('keeps a raw frame as text', () => {
    useConsoleStore.getState().ingest([{ type: 'raw', text: 'odd' }])
    expect(useConsoleStore.getState().entries[0]).toMatchObject({ kind: 'raw', text: 'odd' })
  })

  it('keeps only the newest BUFFER_CAP entries', () => {
    const frames = Array.from({ length: BUFFER_CAP + 10 }, (_, i) => log(i + 1))
    useConsoleStore.getState().ingest(frames)
    const entries = useConsoleStore.getState().entries
    expect(entries).toHaveLength(BUFFER_CAP)
    expect(entries[0].kind === 'log' && entries[0].record.seq).toBe(11)
  })

  it('collapses the expanded line on reset and toggles it otherwise', () => {
    const store = useConsoleStore.getState()
    store.ingest([log(1)])
    const id = useConsoleStore.getState().entries[0].id
    store.toggleExpanded(id)
    expect(useConsoleStore.getState().expandedId).toBe(id)
    store.toggleExpanded(id)
    expect(useConsoleStore.getState().expandedId).toBeNull()
    store.toggleExpanded(id)
    store.ingest([ready(0, true)])
    expect(useConsoleStore.getState().expandedId).toBeNull()
  })
})
