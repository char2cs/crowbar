import { useMemo } from 'react'
import { useShallow } from 'zustand/react/shallow'
import { subagentAction } from '@/features/agent/hooks/use-subagent-action'
import { useChatStore } from '@/features/agent/stores/chat-store'
import type { SubagentItem } from '@/features/agent/stores/lib/transcript-items'
import type { TrayRow } from '@/features/agent/trays/lib/tray-rows'

const FIELDS = 4

/** The work the agent has going, for the bottom tray: today its running subagents. */
export function useWorkRows(chatId: string): TrayRow[] {
  // Flat strings, so the selector's shallow compare holds while unrelated rows stream in.
  const flat = useChatStore(
    useShallow((s) => {
      const feed = s.feeds[chatId]
      return (feed?.facts.liveSubagents ?? []).flatMap((id) => {
        const card = feed?.items.get(id) as SubagentItem
        return [id, card.title || card.type, subagentAction(feed, card.scope), card.startedAt]
      })
    }),
  )
  return useMemo(() => {
    const rows: TrayRow[] = []
    for (let i = 0; i < flat.length; i += FIELDS) {
      const [id, label, detail, startedAt] = flat.slice(i, i + FIELDS) as [
        string,
        string,
        string,
        string,
      ]
      rows.push({ id, kind: 'subagent', label, detail, status: 'running', startedAt })
    }
    return rows
  }, [flat])
}
