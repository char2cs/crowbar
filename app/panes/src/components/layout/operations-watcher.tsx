import { useEffect } from 'react'
import { toast } from '@/features/window/stores/toast-store'
import {
  operationsState,
  subscribeOperations,
  type Operation,
  type OperationState,
} from '@/core/entities/operations'
import { reportDaemonRefusal } from '@/lib/ws/connection-store'

function reason(op: Operation): string | undefined {
  if (op.error?.code !== 'partial') {
    return (op.error ?? op.items.find((item) => item.error)?.error)?.message
  }
  const failed = op.items.flatMap((item) =>
    item.error ? [`${item.label} — ${item.error.message}`] : [],
  )
  return failed.length ? `${op.error.message}: ${failed.join('; ')}` : op.error.message
}

function isNews(op: Operation, before: OperationState | undefined, mounted: boolean): boolean {
  if (op.state !== 'failed' || op.dismissed || before === 'failed') return false
  return before !== undefined || (mounted && !op.snapshot)
}

export function OperationsWatcher() {
  useEffect(() => {
    const seen = new Map<string, OperationState>()
    const look = (ops: Record<string, Operation>, mounted: boolean) => {
      for (const op of Object.values(ops)) {
        const before = seen.get(op.id)
        seen.set(op.id, op.state)
        if (!isNews(op, before, mounted)) continue
        if (op.error && reportDaemonRefusal(op.error)) continue
        toast.show({
          key: op.id,
          type: 'error',
          message: `${op.label} failed`,
          description: reason(op),
        })
      }
    }
    look(operationsState().ops, false)
    return subscribeOperations(() => look(operationsState().ops, true))
  }, [])
  return null
}
