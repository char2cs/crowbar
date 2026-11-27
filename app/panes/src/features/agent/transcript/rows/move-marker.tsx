import { useState } from 'react'
import { Button } from '@/components/ui/button'
import type { MoveBody } from '@/features/agent/api/chat-rows'
import { branchOutChat, setChatPlacement } from '@/features/agent/api/agent-chat-commands'
import { useChatField } from '@/features/agent/hooks/use-chat-entity'
import { report } from '@/lib/errors/report'

export function moveLabel(move: MoveBody | undefined): string {
  if (move?.branchOut) return branchOutLabel(move)
  const where = move?.label || 'another workspace'
  switch (move?.phase) {
    case 'done':
      return move.toDir ? `Moved to ${where} — now working in ${move.toDir}` : `Moved to ${where}`
    case 'failed':
      return `Move to ${where} failed${move.reason ? `: ${move.reason}` : ''}`
    case 'cancelled':
      return 'Move cancelled'
    default:
      return `Moving to ${where} after this turn`
  }
}

function branchOutLabel(move: MoveBody): string {
  switch (move.phase) {
    case 'done':
      return move.toDir
        ? `Now on branch ${move.branch} — working in ${move.toDir}`
        : `Now on branch ${move.branch}`
    case 'failed':
      return `Making a branch failed${move.reason ? `: ${move.reason}` : ''}`
    case 'cancelled':
      return 'Branch cancelled'
    default:
      return 'Becoming a branch after this turn'
  }
}

export function MoveRetry({ chatId, move }: { chatId: string; move: MoveBody }) {
  const [sent, setSent] = useState(false)
  const runsIn = useChatField(chatId, (c) => c?.workspaceId || null)
  const waiting = useChatField(chatId, (c) => !!c?.pendingMove)
  const to = move.to
  if (move.phase !== 'failed' || (!to && !move.branchOut) || !runsIn || waiting) return null
  const retry = async () => {
    setSent(true)
    try {
      if (move.branchOut) await branchOutChat(runsIn, chatId, move.branch ?? '')
      else
        await setChatPlacement(runsIn, chatId, {
          parentId: move.parentId ?? '',
          ...(move.order !== undefined && { order: move.order }),
          workspaceId: to!,
          ...(move.label && { label: move.label }),
        })
    } catch (err) {
      setSent(false)
      report(err, { action: 'Move' })
    }
  }
  return (
    <div className="marker-actions">
      <Button size="sm" variant="secondary" disabled={sent} onClick={() => void retry()}>
        Retry
      </Button>
    </div>
  )
}
