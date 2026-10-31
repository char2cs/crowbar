import { mapChoice } from '@/features/agent/api/agent-chat-activity'
import type { LedgerRow, RowKind } from '@/features/agent/api/chat-rows'
import type { FeedFold } from '@/features/agent/stores/lib/fold-rows'
import {
  askItemId,
  toolItemId,
  toolStatus,
  type AskItem,
  type ToolItem,
} from '@/features/agent/stores/lib/transcript-items'
import { toolFamily } from '@/features/agent/transcript/lib/tool-family'

type Apply<K extends RowKind> = (fold: FeedFold, row: LedgerRow<K>) => void

export const markOutput = (fold: FeedFold, row: LedgerRow) => {
  if (row.scope || !row.turnId || fold.facts.output[row.turnId]) return
  fold.record('output')[row.turnId] = true
}

export const applyToolCall: Apply<'tool_call'> = (fold, row) => {
  const scope = row.scope ?? ''
  const id = toolItemId(row.body.toolId)
  const known = fold.item<ToolItem>(id)
  const body = known ? { ...known.body, ...row.body } : row.body
  const status = toolStatus(row.body, known?.status)
  fold.put(
    known
      ? { ...known, body, status }
      : {
          kind: 'tool',
          id,
          seq: row.seq,
          turnId: row.turnId,
          at: row.at,
          toolId: row.body.toolId,
          scope,
          body,
          status,
          family: toolFamily(body),
        },
  )
  markOutput(fold, row)
  if (!known) fold.place(id, scope)
}

export const applyAsk: Apply<'ask'> = (fold, row) => {
  const scope = row.scope ?? ''
  const body = row.body
  const id = askItemId(body.askId)
  const choice = mapChoice({
    id: body.askId,
    turnId: row.turnId ?? '',
    seq: row.seq,
    kind: body.kind ?? body.event,
    toolName: body.toolName,
    title: body.title,
    question: body.question,
    mode: body.mode,
    multi: body.multi,
    options: body.options ?? [],
    questions: body.questions,
    schema: body.schema === undefined ? undefined : JSON.stringify(body.schema),
    pending: true,
    answerable: body.answerable,
    at: row.at,
  })
  const known = fold.item<AskItem>(id)
  const tool = body.toolId ? fold.item<ToolItem>(toolItemId(body.toolId)) : undefined
  fold.put({
    kind: 'ask',
    id,
    seq: known?.seq ?? row.seq,
    turnId: row.turnId,
    at: row.at,
    scope,
    choice,
    toolTarget: body.toolTarget || tool?.body.target || '',
    toolId: body.toolId,
  })
  if (!fold.facts.openAsks.includes(id)) fold.set('openAsks', [...fold.facts.openAsks, id])
  if (!known) fold.place(id, scope)
}

export const applyAskResolved: Apply<'ask_resolved'> = (fold, row) => {
  const id = askItemId(row.body.askId)
  const known = fold.item<AskItem>(id)
  if (known) {
    const choice = {
      ...known.choice,
      pending: false,
      resolution: row.body.resolution,
      resolvedAt: row.at,
      answeredOptionIds: row.body.optionIds,
    }
    fold.put({ ...known, choice })
    // A denied tool never runs, so no post or fail row will settle it.
    const picked = new Set(row.body.optionIds)
    const denied =
      row.body.resolution === 'refused' ||
      known.choice.options.some((o) => o.kind === 'deny' && picked.has(o.id))
    const tool = denied && known.toolId ? fold.item<ToolItem>(toolItemId(known.toolId)) : undefined
    if (tool?.status === 'running') fold.put({ ...tool, status: 'declined' })
  }
  const openAsks = fold.facts.openAsks.filter((other) => other !== id)
  if (openAsks.length !== fold.facts.openAsks.length) fold.set('openAsks', openAsks)
}
