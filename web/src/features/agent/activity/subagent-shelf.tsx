import { useCallback, useLayoutEffect, useRef, useState } from 'react'
import { useShallow } from 'zustand/react/shallow'
import { SubagentIcon } from '@/features/agent/shared/agent-icons'
import { useTextClock } from '@/features/agent/activity/lib/text-clock'
import { useSubagentAction } from '@/features/agent/hooks/use-subagent-action'
import { useChatStore } from '@/features/agent/stores/chat-store'
import type { SubagentItem } from '@/features/agent/stores/lib/transcript-items'
import { fitShelf, type ShelfToken } from '@/features/agent/activity/lib/shelf-fit'
import { cn } from '@/lib/utils'

/** Rough px per character of a token's label, for the pre-layout estimate. */
const CHAR_PX = 6.2
const TOKEN_CHROME_PX = 32
const CLOCK_CHARS = 4

function estimate(token: ShelfToken, dense: boolean): number {
  const name = dense ? '' : (token.agentType ?? '')
  return TOKEN_CHROME_PX + (name.length + CLOCK_CHARS) * CHAR_PX
}

function Token({ chatId, card, dense }: { chatId: string; card: SubagentItem; dense: boolean }) {
  const action = useSubagentAction(chatId, card.scope)
  const clockRef = useTextClock(card.startedAt)
  return (
    <span className="tok">
      <i />
      {!dense && <span className="ty">{card.title || card.type}</span>}
      {!dense && <span className="act">{action}</span>}
      <b data-testid="agent-subagent-clock" ref={clockRef} />
    </span>
  )
}

/** The running subagents above the composer: each one's title, current action
 *  and clock. It sheds detail as a fan-out widens (`fitShelf`); the count and
 *  the clocks never drop. */
export function SubagentShelf({ chatId }: { chatId: string }) {
  const lineRef = useRef<HTMLSpanElement>(null)
  const [width, setWidth] = useState(0)
  const running = useChatStore(
    useShallow((s) => {
      const feed = s.feeds[chatId]
      return (feed?.facts.liveSubagents ?? []).map((id) => feed?.items[id] as SubagentItem)
    }),
  )

  useLayoutEffect(() => {
    const node = lineRef.current
    if (!node) return
    const measure = () => setWidth(node.clientWidth)
    measure()
    const observer = new ResizeObserver(measure)
    observer.observe(node)
    return () => observer.disconnect()
  }, [])

  const measure = useCallback(estimate, [])

  if (running.length === 0) return null

  const byId = new Map(running.map((card) => [card.id, card]))
  const tokens: ShelfToken[] = running.map((card) => ({
    id: card.id,
    agentType: card.title || card.type,
    elapsed: 0,
  }))
  const layout = fitShelf(tokens, width, measure)

  return (
    <div
      className={cn('subbar', layout.dense && 'dense')}
      data-testid="agent-subagent-shelf"
      aria-label={`${running.length} ${running.length === 1 ? 'subagent' : 'subagents'} running`}
    >
      <span className="subhd">
        <SubagentIcon size={12} />
        <b>{running.length}</b>&nbsp;{running.length === 1 ? 'subagent' : 'subagents'}
      </span>
      <span className="subline" ref={lineRef}>
        {layout.shown.map((token) => (
          <Token key={token.id} chatId={chatId} card={byId.get(token.id)!} dense={layout.dense} />
        ))}
      </span>
      {layout.overflow > 0 && <span className="submore">+{layout.overflow}</span>}
    </div>
  )
}
