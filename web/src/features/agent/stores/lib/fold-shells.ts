import type { LedgerRow } from '@/features/agent/api/chat-rows'
import type { FeedFold } from '@/features/agent/stores/lib/fold-rows'
import {
  shellItemId,
  type ChatFeed,
  type ShellItem,
} from '@/features/agent/stores/lib/transcript-items'

/** A shell starts on its row, shows its newest line while it runs and ends once, by the
 *  first end the daemon records. */
export function applyShell(fold: FeedFold, row: LedgerRow<'shell'>) {
  if (row.scope) return
  const { phase, shellId } = row.body
  const id = shellItemId(shellId)
  const known = fold.item<ShellItem>(id)
  if (phase === 'start' && !known) {
    const { kind, command, label, cwd, ref, persistent, timeoutMs } = row.body
    fold.put({
      kind: 'shell',
      shellKind: kind ?? 'shell',
      shellId,
      id,
      seq: row.seq,
      turnId: row.turnId,
      at: row.at,
      command: command ?? '',
      label: label ?? '',
      cwd,
      ref,
      persistent,
      timeoutMs,
      status: 'running',
      startedAt: row.at,
    })
    fold.set('shells', [...fold.facts.shells, id])
  } else if (known?.status === 'running' && phase === 'note') {
    fold.put({ ...known, tail: row.body.tail ?? known.tail })
  } else if (known?.status === 'running' && phase === 'end' && row.body.status) {
    const { status, exitCode, tail, summary } = row.body
    fold.put({
      ...known,
      status,
      exitCode,
      tail: tail ?? known.tail,
      summary,
      endedAt: row.at,
    })
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
