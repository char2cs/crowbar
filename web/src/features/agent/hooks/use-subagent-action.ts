import { useChatStore } from '@/features/agent/stores/chat-store'
import { labelText, liveLabel } from '@/features/agent/transcript/lib/tool-label'

/** What a subagent is doing, from its own rows: its newest running call, else
 *  its newest finished one, else nothing yet. Computed, never stored. */
export function useSubagentAction(chatId: string, scope: string | undefined): string {
  return useChatStore((s) => {
    const feed = s.feeds[chatId]
    const ids = scope ? (feed?.scoped[scope] ?? []) : []
    let settled: string | undefined
    for (let i = ids.length - 1; i >= 0; i--) {
      const item = feed?.items.get(ids[i]!)
      if (item?.kind !== 'tool' || item.family === 'meta' || item.family === 'plan') continue
      if (item.status === 'running') return liveLabel(item)
      settled ??= labelText(item)
    }
    return settled ?? 'Thinking…'
  })
}
