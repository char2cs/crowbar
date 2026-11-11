import type { ReactNode } from 'react'
import type { PlanItem } from '@/features/agent/api/chat-rows'
import { PlanTray } from '@/features/agent/trays/plan-tray'
import { QueuedTray } from '@/features/agent/trays/queued-tray'
import { ResolvedTray } from '@/features/agent/trays/resolved-tray'
import { useTopTray } from '@/features/agent/trays/use-top-tray'

interface TopTrayProps {
  chatId: string
  plan: PlanItem[] | undefined
  /** A failure notice, drawn first; its owner supplies the content. */
  failure?: ReactNode
}

/** Behind the input box's top edge: a failure, the to-do list, queued messages and the answered
 *  requests, in one tray that exists only while one of them has something to show. */
export function TopTray({ chatId, plan, failure }: TopTrayProps) {
  const { steps, queued, resolved } = useTopTray(chatId, plan)
  if (!failure && !steps && queued.length === 0 && resolved.length === 0) return null
  return (
    <div className="tray-top" data-testid="agent-top-tray">
      {failure}
      <PlanTray plan={steps ?? undefined} />
      <QueuedTray chatId={chatId} items={queued} />
      <ResolvedTray choices={resolved} />
    </div>
  )
}
