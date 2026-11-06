import { describe, expect, it } from 'vitest'
import type { ToolCallBody } from '@/features/agent/api/chat-rows'
import { ItemMap } from '@/features/agent/stores/lib/item-map'
import type { ToolItem, TranscriptItem } from '@/features/agent/stores/lib/transcript-items'
import { toolFamily } from '@/features/agent/transcript/lib/tool-family'
import { displayRows, runSentence } from '@/features/agent/transcript/lib/tool-runs'

let n = 0
function tool(
  body: Partial<ToolCallBody>,
  status: ToolItem['status'] = 'ok',
  turnId = 'turn-1',
): ToolItem {
  n += 1
  const full = { toolId: `tool-${n}`, phase: 'post' as const, ...body }
  return {
    kind: 'tool',
    id: `t:tool-${n}`,
    seq: n,
    turnId,
    at: '',
    toolId: full.toolId,
    scope: '',
    body: full,
    status,
    family: toolFamily(full),
  }
}

const text = (id: string): TranscriptItem => ({
  kind: 'text',
  id,
  seq: 0,
  turnId: 'turn-1',
  at: '',
  messageId: id,
  chunks: {},
  text: 'hi',
  settled: true,
  phase: '',
})

function rowsOf(list: TranscriptItem[]) {
  const items = ItemMap.empty.edit()
  for (const item of list) items.set(item)
  return displayRows(
    list.map((item) => item.id),
    items.finish(),
  )
}

describe('displayRows', () => {
  it('folds consecutive reads and searches of one turn into one run with a stable key', () => {
    const a = tool({ kind: 'read', target: '/a' })
    const b = tool({ kind: 'search', target: 'x' })
    expect(rowsOf([a])).toEqual([{ id: `g:${a.id}`, members: [a.id] }])
    const grown = rowsOf([a, b])
    expect(grown).toEqual([{ id: `g:${a.id}`, members: [a.id, b.id] }])
    const other = tool({ kind: 'read', target: '/c' }, 'ok', 'turn-2')
    expect(rowsOf([a, b, other]).map((row) => row.id)).toEqual([`g:${a.id}`, `g:${other.id}`])
  })

  it('breaks a run at text, an edit, a failed command, a subagent or an ask', () => {
    const breakers: TranscriptItem[] = [
      text('m:1'),
      tool({ kind: 'edit', target: '/e' }),
      tool({ kind: 'execute', target: 'false' }, 'error'),
      tool({ kind: 'subagent', name: 'Agent' }),
      {
        kind: 'ask',
        id: 'a:q',
        seq: 0,
        at: '',
        scope: '',
        toolTarget: '',
        choice: { id: 'q', kind: 'question', options: [], pending: true } as never,
      },
    ]
    for (const breaker of breakers) {
      const before = tool({ kind: 'read', target: '/a' })
      const after = tool({ kind: 'read', target: '/b' })
      expect(rowsOf([before, breaker, after])).toEqual([
        { id: `g:${before.id}`, members: [before.id] },
        { id: breaker.id },
        { id: `g:${after.id}`, members: [after.id] },
      ])
    }
  })

  // A memoized row skips only while its props keep their identity.
  it('keeps the rows a newer call leaves as they were', () => {
    const a = tool({ kind: 'read', target: '/a' })
    const b = tool({ kind: 'search', target: 'x' })
    const edit = tool({ kind: 'edit', target: '/a' })
    const items = ItemMap.empty.edit()
    for (const item of [a, b, edit]) items.set(item)
    const map = items.finish()
    const before = displayRows([a.id, b.id, edit.id], map)
    const after = displayRows([a.id, b.id, edit.id, 'later'], map, before)
    expect(after[0]).toBe(before[0])
    expect(after[1]).toBe(before[1])
  })

  it('hides meta and plan tool rows', () => {
    const a = tool({ kind: 'read', target: '/a' })
    const meta = tool({ kind: 'meta', name: 'ToolSearch' })
    const plan = tool({ kind: 'plan', name: 'TaskUpdate' })
    const b = tool({ kind: 'read', target: '/b' })
    expect(rowsOf([a, meta, plan, b])).toEqual([{ id: `g:${a.id}`, members: [a.id, b.id] }])
  })
})

describe('runSentence', () => {
  it('counts the run by family, live and settled', () => {
    const reads = [tool({ kind: 'read' }, 'running'), tool({ kind: 'read' }, 'running')]
    const search = tool({ kind: 'search' }, 'running')
    expect(runSentence([...reads, search], true)).toBe('Reading 2 files, running 1 search…')
    const fetch = tool({ kind: 'fetch' })
    expect(runSentence([reads[0]!, search, fetch], false)).toBe(
      'Read 1 file, ran 1 search, fetched 1 page',
    )
  })
})
