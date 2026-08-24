import { useCallback, useEffect, useLayoutEffect, useRef, useState } from 'react'
import { SubagentIcon } from '@/features/agent/shared/agent-icons'
import type { AgentActivity } from '@/features/agent/api/agent-api'
import { fitShelf, formatElapsed, type ShelfToken } from '@/features/agent/activity/lib/shelf-fit'
import { cn } from '@/lib/utils'

/** Rough px per character of a token's label, for the pre-layout estimate. */
const CHAR_PX = 6.2
const TOKEN_CHROME_PX = 32

function estimate(token: ShelfToken, dense: boolean): number {
  const name = dense ? '' : (token.agentType ?? '')
  const clock = formatElapsed(token.elapsed)
  return TOKEN_CHROME_PX + (name.length + clock.length) * CHAR_PX
}

/**
 * How many subagents are running, and for how long.
 *
 * That is the WHOLE payload — `AgentSubagent` carries an id, an optional type
 * and two timestamps, and nothing else exists to show. It is not actionable and
 * it never pretends to be: there is no way to inspect a subagent from here,
 * because Crowbar is never told what one is doing.
 *
 * It sheds detail in a fixed order as a fan-out widens (see `fitShelf`), and the
 * count and clocks are the two things that never drop.
 */
export function SubagentShelf({ activity }: { activity: AgentActivity }) {
  const lineRef = useRef<HTMLSpanElement>(null)
  const [width, setWidth] = useState(0)
  const [now, setNow] = useState(() => Date.now())

  const running = activity.subagents.filter((subagent) => !subagent.endedAt)

  useLayoutEffect(() => {
    const node = lineRef.current
    if (!node) return
    const measure = () => setWidth(node.clientWidth)
    measure()
    const observer = new ResizeObserver(measure)
    observer.observe(node)
    return () => observer.disconnect()
  }, [])

  useEffect(() => {
    if (running.length === 0) return
    const timer = window.setInterval(() => setNow(Date.now()), 1000)
    return () => window.clearInterval(timer)
  }, [running.length])

  const measure = useCallback(estimate, [])

  if (running.length === 0) return null

  const tokens: ShelfToken[] = running.map((subagent) => ({
    id: subagent.id,
    agentType: subagent.agentType,
    elapsed: Math.max(0, Math.round((now - Date.parse(subagent.startedAt)) / 1000)),
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
          <span className="tok" key={token.id}>
            <i />
            {!layout.dense && token.agentType && <span className="ty">{token.agentType}</span>}
            <b>{formatElapsed(token.elapsed)}</b>
          </span>
        ))}
      </span>
      {layout.overflow > 0 && <span className="submore">+{layout.overflow}</span>}
    </div>
  )
}
