import { describe, expect, it } from 'vitest'
import { formatTrayClock, windowRows, type TrayRow } from '@/features/agent/trays/lib/tray-rows'

const row = (
  id: string,
  kind: TrayRow['kind'] = 'subagent',
  status: TrayRow['status'] = 'running',
): TrayRow => ({ id, kind, label: id, detail: '', status, startedAt: '2026-10-01T12:00:00Z' })

const ids = (entries: ReturnType<typeof windowRows>['entries']) =>
  entries.map((entry) => (entry.type === 'row' ? entry.row.id : `# ${entry.text}`))

describe('formatTrayClock', () => {
  it('writes seconds, then minutes with padded seconds, then hours', () => {
    expect(formatTrayClock(58)).toBe('58s')
    expect(formatTrayClock(102)).toBe('1m 42s')
    expect(formatTrayClock(363)).toBe('6m 03s')
    expect(formatTrayClock(3900)).toBe('1h 05m')
  })
})

describe('windowRows', () => {
  it('shows up to four rows as they are', () => {
    const view = windowRows([row('a'), row('b'), row('c'), row('d')], false)
    expect(ids(view.entries)).toEqual(['a', 'b', 'c', 'd'])
    expect(view.more).toBeNull()
    expect(view.canCollapse).toBe(false)
  })

  it('shows no label for the one kind left once the rest fold away', () => {
    const rows = [row('a'), row('b'), row('c'), row('sh', 'shell'), row('mo', 'monitor')]
    const view = windowRows(rows, false)
    expect(ids(view.entries)).toEqual(['a', 'b', 'c'])
    expect(view.more).toEqual({ hidden: 2, running: 2 })
  })

  it('folds past four into three rows and a group row counting what runs', () => {
    const rows = [row('a'), row('b'), row('c'), row('d', 'subagent', 'done'), row('e')]
    const view = windowRows(rows, false)
    expect(ids(view.entries)).toEqual(['a', 'b', 'c'])
    expect(view.more).toEqual({ hidden: 2, running: 1 })
    expect(view.canCollapse).toBe(false)
  })

  it('shows every row once expanded and offers to fold again', () => {
    const rows = ['a', 'b', 'c', 'd', 'e'].map((id) => row(id))
    const view = windowRows(rows, true)
    expect(ids(view.entries)).toEqual(['a', 'b', 'c', 'd', 'e'])
    expect(view.more).toBeNull()
    expect(view.canCollapse).toBe(true)
  })

  it('labels the groups only when more than one kind shows, in the order subagents, shells, monitors', () => {
    const view = windowRows([row('mo', 'monitor'), row('sh', 'shell'), row('ag')], false)
    expect(ids(view.entries)).toEqual(['# Subagents', 'ag', '# Shells', 'sh', '# Monitors', 'mo'])
    expect(ids(windowRows([row('mo', 'monitor'), row('sh', 'shell')], false).entries)).toEqual([
      '# Shells',
      'sh',
      '# Monitors',
      'mo',
    ])
    expect(ids(windowRows([row('mo', 'monitor')], false).entries)).toEqual(['mo'])
    expect(ids(windowRows([row('ag')], false).entries)).toEqual(['ag'])
    expect(ids(windowRows([row('sh', 'shell')], false).entries)).toEqual(['sh'])
  })
})
