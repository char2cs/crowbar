import { useMemo } from 'react'
import { useShallow } from 'zustand/react/shallow'
import { useChatShells } from '@/features/agent/hooks/use-chat-shells'
import { subagentAction } from '@/features/agent/hooks/use-subagent-action'
import { useChatStore } from '@/features/agent/stores/chat-store'
import type { SubagentItem } from '@/features/agent/stores/lib/transcript-items'
import { shellRow } from '@/features/agent/trays/lib/shell-rows'
import type { TrayRow } from '@/features/agent/trays/lib/tray-rows'

const FIELDS = 6

const ENDED: Record<string, TrayRow['status']> = {
  done: 'done',
  failed: 'failed',
  interrupted: 'stopped',
  abandoned: 'stopped',
}

/** How a subagent ended, in the tray's words: "done · 14 tool uses", "failed · could not start". */
function endNote(card: SubagentItem): string {
  const uses = card.usage?.toolUses
  const said = card.status === 'failed' ? card.result?.trim().split('\n')[0] : undefined
  const word = card.status === 'done' || card.status === 'failed' ? card.status : 'stopped'
  const extra = said || (uses ? `${uses} tool ${uses === 1 ? 'use' : 'uses'}` : '')
  return extra ? `${word} · ${extra}` : word
}

/** The work the agent has going, for the bottom tray: the turn's subagents, running ones first
 *  and the ended ones kept beside them while any still runs, then its background shells and
 *  monitors, running ones first. */
export function useWorkRows(chatId: string): TrayRow[] {
  // Flat strings, so the selector's shallow compare holds while unrelated rows stream in.
  const flat = useChatStore(
    useShallow((s) => {
      const feed = s.feeds[chatId]
      const live = (feed?.facts.liveSubagents ?? []).map(
        (id) => feed?.items.get(id) as SubagentItem,
      )
      const turn = live.at(-1)?.turnId
      const ended = live.length
        ? (feed?.order ?? [])
            .map((id) => feed?.items.get(id))
            .filter(
              (item): item is SubagentItem =>
                item?.kind === 'subagent' && item.turnId === turn && !!ENDED[item.status],
            )
        : []
      return [
        ...live.flatMap((card) => [
          card.id,
          card.title || card.type,
          subagentAction(feed, card.scope),
          card.startedAt,
          'running',
          '',
        ]),
        ...ended.flatMap((card) => [
          card.id,
          card.title || card.type,
          endNote(card),
          card.startedAt,
          card.status,
          card.endedAt ?? '',
        ]),
      ]
    }),
  )
  const shells = useChatShells(chatId)
  return useMemo(() => {
    const rows: TrayRow[] = []
    for (let i = 0; i < flat.length; i += FIELDS) {
      const [id, label, detail, startedAt, status, endedAt] = flat.slice(
        i,
        i + FIELDS,
      ) as string[] as [string, string, string, string, string, string]
      rows.push({
        id,
        kind: 'subagent',
        label,
        detail,
        status: ENDED[status] ?? 'running',
        startedAt,
        endedAt: endedAt || undefined,
      })
    }
    const running = (row: TrayRow) => row.status === 'running'
    const tracked = shells.map(shellRow)
    return [...rows, ...tracked.filter(running), ...tracked.filter((row) => !running(row))]
  }, [flat, shells])
}
