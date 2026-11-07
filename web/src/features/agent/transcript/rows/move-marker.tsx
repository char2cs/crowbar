import { useState } from 'react'
import { Button } from '@/components/ui/button'
import type { MoveBody } from '@/features/agent/api/chat-rows'
import {
  answerChatMove,
  branchOutChat,
  setChatPlacement,
} from '@/features/agent/api/agent-chat-commands'
import { useChatField } from '@/features/agent/hooks/use-chat-entity'
import { report } from '@/lib/errors/report'

/** A move's line, in the words of where it went. */
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

/** A thread made a branch of its own: named by the branch it became. */
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
    case 'requested':
      return move.awaitsConsent
        ? 'The agent asks for a branch'
        : 'Becoming a branch after this turn'
    default:
      return 'Becoming a branch after this turn'
  }
}

/** Asks for a failed move again, as its request named it, while no other move is waiting. */
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
          // The ledger omits a root parent; unsent, the daemon would keep the one it has here.
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

/** The agent's request for a move, held until the person allows or denies it; gone once answered. */
export function MoveConsent({ chatId, move }: { chatId: string; move: MoveBody }) {
  const [sent, setSent] = useState(false)
  const [reason, setReason] = useState('')
  const runsIn = useChatField(chatId, (c) => c?.workspaceId || null)
  const asking = useChatField(
    chatId,
    (c) => c?.pendingMove?.requestId === move.requestId && !!c.pendingMove.awaitsConsent,
  )
  if (!asking || !runsIn) return null
  const answer = async (allow: boolean) => {
    setSent(true)
    try {
      await answerChatMove(runsIn, chatId, {
        requestId: move.requestId,
        allow,
        reason: allow ? '' : reason.trim(),
      })
    } catch (err) {
      setSent(false)
      report(err, { action: allow ? 'Allow' : 'Deny' })
    }
  }
  const what = move.branchOut
    ? `make this conversation ${move.branch ? `branch ${move.branch}` : 'a branch of its own'}`
    : `move this conversation to ${move.label || 'another workspace'}`
  return (
    <div className="marker-actions flex-col items-stretch gap-2">
      <p className="text-sm">
        The agent asks to {what}. It happens when its turn ends; edits not committed stay behind.
      </p>
      <div className="flex items-center gap-2">
        <input
          type="text"
          value={reason}
          placeholder="Reason (optional)"
          disabled={sent}
          onChange={(e) => setReason(e.target.value)}
          className="min-w-0 flex-1 rounded-md border border-border bg-transparent px-2 py-1 text-sm"
        />
        <Button size="sm" variant="secondary" disabled={sent} onClick={() => void answer(false)}>
          Deny
        </Button>
        <Button size="sm" disabled={sent} onClick={() => void answer(true)}>
          Allow
        </Button>
      </div>
    </div>
  )
}
