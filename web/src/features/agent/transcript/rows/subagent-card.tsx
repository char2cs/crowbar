import { memo, useMemo, useState } from 'react'
import { CaretRightIcon, CheckIcon, CircleDashedIcon, StopIcon, XIcon } from '@phosphor-icons/react'
import type { Icon } from '@phosphor-icons/react'
import { FlickerSpinner } from '@/components/ui/flicker-spinner'
import { useTextClock } from '@/features/agent/activity/lib/text-clock'
import { useChatItem, useChatItems } from '@/features/agent/hooks/use-chat-feed'
import { useSubagentAction } from '@/features/agent/hooks/use-subagent-action'
import { formatTrayClock } from '@/features/agent/trays/lib/tray-rows'
import { SubagentIcon } from '@/features/agent/shared/agent-icons'
import { useChatStore } from '@/features/agent/stores/chat-store'
import { ItemMap } from '@/features/agent/stores/lib/item-map'
import type { SubagentItem } from '@/features/agent/stores/lib/transcript-items'
import { displayRows } from '@/features/agent/transcript/lib/tool-runs'
import { MarkdownMessageStatic } from '@/features/agent/transcript/plate/markdown-message-static'
import { TextRow } from '@/features/agent/transcript/rows/text-row'
import { AskRow, ToolRow } from '@/features/agent/transcript/rows/tool-row'
import { ToolRunRow } from '@/features/agent/transcript/rows/tool-run-row'

const NO_IDS: string[] = []

/** A subagent that ended any way but done says how, as a tool call's outcome does. */
const OUTCOMES: Partial<
  Record<SubagentItem['status'], { kind: string; text: string; Glyph: Icon }>
> = {
  failed: { kind: 'failed', text: 'Failed', Glyph: XIcon },
  interrupted: { kind: 'stopped', text: 'Stopped', Glyph: StopIcon },
  abandoned: { kind: 'unavailable', text: 'Result unavailable', Glyph: CircleDashedIcon },
}

const tokens = (n: number) => (n >= 1000 ? `${Math.round(n / 1000)}k` : String(n))

function spent(card: SubagentItem): string {
  const parts: string[] = []
  const uses = card.usage?.toolUses
  if (uses) parts.push(`${uses} tool ${uses === 1 ? 'use' : 'uses'}`)
  if (card.usage?.tokens) parts.push(`${tokens(card.usage.tokens)} tokens`)
  return parts.join(' · ')
}

function took(card: SubagentItem): string {
  const ms =
    card.usage?.durationMs ??
    (card.endedAt ? Date.parse(card.endedAt) - Date.parse(card.startedAt) : 0)
  return ms > 0 ? formatTrayClock(Math.round(ms / 1000)) : ''
}

/** The subagent's own rows, by the same components as the main transcript. */
function Work({ chatId, scope }: { chatId: string; scope: string }) {
  const ids = useChatStore((s) => s.feeds[chatId]?.scoped[scope] ?? NO_IDS)
  const items = useChatItems(chatId)
  const rows = useMemo(() => displayRows(ids, items ?? ItemMap.empty), [ids, items])
  return (
    <div className="subagent-work">
      {rows.map((row) => {
        if (row.members) {
          return <ToolRunRow key={row.id} chatId={chatId} id={row.id} members={row.members} />
        }
        const item = items?.get(row.id)
        switch (item?.kind) {
          case 'tool':
            return <ToolRow key={row.id} chatId={chatId} item={item} />
          case 'text':
            return (
              <TextRow
                key={row.id}
                chatId={chatId}
                item={item}
                firstReply={false}
                turnbar={false}
              />
            )
          case 'ask':
            return <AskRow key={row.id} item={item} />
          case 'subagent':
            return <SubagentCard key={row.id} chatId={chatId} id={row.id} />
          default:
            return null
        }
      })}
    </div>
  )
}

function SubagentCardBody({ chatId, id }: { chatId: string; id: string }) {
  const card = useChatItem<SubagentItem>(chatId, id)
  const [open, setOpen] = useState(false)
  const action = useSubagentAction(chatId, card?.scope)
  const live = card?.status === 'starting' || card?.status === 'running'
  const clockRef = useTextClock(live ? card?.startedAt : undefined, formatTrayClock)
  if (card?.kind !== 'subagent') return null
  const usage = live ? '' : spent(card)
  const outcome = OUTCOMES[card.status]
  const head = (
    <>
      {live ? (
        <FlickerSpinner className="size-3" />
      ) : outcome ? (
        <outcome.Glyph size={12} className="tool-glyph" />
      ) : (
        <CheckIcon size={12} className="tool-glyph" />
      )}
      <SubagentIcon size={12} />
      <span className="subagent-title">{card.title || card.type || 'Subagent'}</span>
      {card.type && card.title && <span className="subagent-type">{card.type}</span>}
      {(live || took(card)) && (
        <span className="subagent-clock" ref={clockRef}>
          {live ? '' : took(card)}
        </span>
      )}
      {outcome && (
        <span className="tool-call-state" data-outcome={outcome.kind}>
          {outcome.text}
          {card.status === 'failed' && !usage && !took(card) && ' · could not start'}
        </span>
      )}
    </>
  )
  return (
    <article
      className="row subagent-card"
      data-row-id={card.id}
      data-testid="agent-subagent-card"
      data-status={card.status}
    >
      <div className="assistant">
        {card.scope ? (
          // The whole header is the toggle; the caret only shows which way it points.
          <button
            type="button"
            className="subagent-head"
            title={usage || undefined}
            aria-expanded={open}
            onClick={() => setOpen((o) => !o)}
          >
            {head}
            <span className="tool-row-toggle" aria-hidden="true">
              <CaretRightIcon size={12} />
            </span>
          </button>
        ) : (
          <div className="subagent-head" title={usage || undefined}>
            {head}
          </div>
        )}
        {live && (
          <p className="subagent-action" data-testid="agent-subagent-action">
            ▸ {action}
          </p>
        )}
        {live && card.parentWaiting && <p className="subagent-waiting">Parent waiting…</p>}
        {open && card.scope && <Work chatId={chatId} scope={card.scope} />}
        {open && card.result && (
          <div className="subagent-result" data-testid="agent-subagent-result">
            <MarkdownMessageStatic variant="chat">{card.result}</MarkdownMessageStatic>
          </div>
        )}
      </div>
    </article>
  )
}

export const SubagentCard = memo(SubagentCardBody)
