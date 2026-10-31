import { useEffect } from 'react'
import { toast } from '@/features/window/stores/toast-store'
import {
  operationsState,
  subscribeOperations,
  type Operation,
  type OperationState,
} from '@/lib/store/operations'

function reason(op: Operation): string | undefined {
  return (op.error ?? op.items.find((item) => item.error)?.error)?.message
}

// A failure is news when it was seen running, or arrives failed on a live
// frame (daemon-started work); one held at mount or sent in a snapshot is not.
function isNews(op: Operation, before: OperationState | undefined, mounted: boolean): boolean {
  if (op.state !== 'failed' || op.dismissed || before === 'failed') return false
  return before !== undefined || (mounted && !op.snapshot)
}

/** Toasts each operation that fails while this window watches it, once. */
export function OperationsWatcher() {
  useEffect(() => {
    const seen = new Map<string, OperationState>()
    const look = (ops: Record<string, Operation>, mounted: boolean) => {
      for (const op of Object.values(ops)) {
        const before = seen.get(op.id)
        seen.set(op.id, op.state)
        if (!isNews(op, before, mounted)) continue
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
