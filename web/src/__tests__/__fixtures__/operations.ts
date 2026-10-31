import { entityStore } from '@/core/entities/entity-store'
import { forgetLocalOperation, useLocalOperationsStore } from '@/lib/store/local-operations'
import type { Operation } from '@/lib/store/operations'

let v = 0

/** The daemon's row for `op` as a live projects-scope frame leaves it. */
export function streamOperation(op: Operation): void {
  entityStore.setState((s) => ({
    byKind: { ...s.byKind, operation: { ...s.byKind.operation, [op.id]: { v: ++v, data: op } } },
  }))
  forgetLocalOperation(op.id, op.version)
}

/** No operation anywhere. */
export function resetOperations(): void {
  entityStore.setState({ byKind: { operation: {} }, members: {}, fresh: {}, synced: {} })
  useLocalOperationsStore.setState({ ops: {} })
}
