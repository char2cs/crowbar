import type { LedgerRow } from '@/features/agent/api/chat-rows'
import type { FeedFold } from '@/features/agent/stores/lib/fold-rows'
import {
  shellItemId,
  type ChatFeed,
  type ShellItem,
} from '@/features/agent/stores/lib/transcript-items'

/** A shell starts on its row and ends once, by the first end the daemon records. */
export function applyShell(fold: FeedFold, row: LedgerRow<'shell'>) {
  if (row.scope) return
  const { phase, shellId, command, label, status } = row.body
  const id = shellItemId(shellId)
  const known = fold.item<ShellItem>(id)
  if (phase === 'start' && !known) {
    fold.put({
      kind: 'shell',
      shellId,
      id,
      seq: row.seq,
      turnId: row.turnId,
      at: row.at,
      command: command ?? '',
      label: label ?? '',
      status: 'running',
      startedAt: row.at,
    })
    fold.set('shells', [...fold.facts.shells, id])
  } else if (phase === 'end' && known?.status === 'running' && status) {
    fold.put({ ...known, status, endedAt: row.at })
  }
}

/** The CLI that ran them is gone, and takes its background commands with it. */
export function stopShells(fold: FeedFold, row: LedgerRow) {
  for (const id of fold.facts.shells) {
    const shell = fold.item<ShellItem>(id)
    if (shell?.status === 'running') fold.put({ ...shell, status: 'stopped', endedAt: row.at })
  }
}

/** The chat's background shells in the order they started. */
export function shellsOf(feed: ChatFeed): ShellItem[] {
  return feed.facts.shells.map((id) => feed.items.get(id) as ShellItem)
}
