import type { AgentProvider } from '@/features/agent/api/agent-providers-api'
import type { PlanItem } from '@/features/agent/api/chat-rows'
import { PlanTray } from '@/features/agent/trays/plan-tray'
import { QueuedTray } from '@/features/agent/trays/queued-tray'
import { SendFailureTray } from '@/features/agent/trays/send-failure-tray'
import { ResolvedTray } from '@/features/agent/trays/resolved-tray'
import { TurnFailureTray, useNewestStop } from '@/features/agent/trays/turn-failure-tray'
import { useTopTray } from '@/features/agent/trays/use-top-tray'
import type { FailureActions } from '@/features/agent/transcript/lib/marker-failure'

interface TopTrayProps {
  chatId: string
  plan: PlanItem[] | undefined
  providers: AgentProvider[]
  /** What the stopped turn's notice can do. */
  actions: FailureActions
  /** Puts an unsent message's text back in the composer. */
  onEdit: (text: string) => void
}

/** Behind the input box's top edge: a failure, the to-do list, queued messages and the answered
 *  requests, in one tray that exists only while one of them has something to show. */
export function TopTray({ chatId, plan, providers, actions, onEdit }: TopTrayProps) {
  const { steps, queued, failed, resolved } = useTopTray(chatId, plan)
  const stop = useNewestStop(chatId)
  if (!failed && !stop && !steps && queued.length === 0 && resolved.length === 0) return null
  return (
    <div className="tray-top" data-testid="agent-top-tray">
      {stop && (
        <TurnFailureTray chatId={chatId} item={stop} providers={providers} actions={actions} />
      )}
      <SendFailureTray chatId={chatId} item={failed} onEdit={onEdit} />
      <PlanTray plan={steps ?? undefined} />
      <QueuedTray chatId={chatId} items={queued} onEdit={onEdit} />
      <ResolvedTray choices={resolved} />
    </div>
  )
}
