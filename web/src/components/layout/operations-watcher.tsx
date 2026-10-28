import { useEffect } from 'react'
import { toast } from '@/features/window/stores/toast-store'
import { useOperationsStore, type Operation, type OperationState } from '@/lib/store/operations'

function reason(op: Operation): string | undefined {
  return (op.error ?? op.items.find((item) => item.error)?.error)?.message
}

/** Toasts each operation that fails while this window watches it, once. One
 *  already failed when first seen was shown before, or happened while away. */
export function OperationsWatcher() {
  useEffect(() => {
    const seen = new Map<string, OperationState>()
    const look = (ops: Record<string, Operation>) => {
      for (const op of Object.values(ops)) {
        const before = seen.get(op.id)
        seen.set(op.id, op.state)
        if (before === undefined || before === 'failed' || op.state !== 'failed' || op.dismissed)
          continue
        toast.show({
          key: op.id,
          type: 'error',
          message: `${op.label} failed`,
          description: reason(op),
        })
      }
    }
    look(useOperationsStore.getState().ops)
    return useOperationsStore.subscribe((s) => look(s.ops))
  }, [])
  return null
}
