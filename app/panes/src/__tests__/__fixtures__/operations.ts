import { entityStore } from '@/core/entities/entity-store'
import { entityKey } from '@/core/stream/host-scope'
import { forgetLocalOperation, useLocalOperationsStore } from '@/lib/store/local-operations'
import type { Operation } from '@/core/entities/operations'

let v = 0

/** `hostId`'s row for `op` as a live projects-scope frame leaves it. */
export function streamOperation(op: Operation, hostId = 'local'): void {
  const key = entityKey(hostId, op.id)
  entityStore.setState((s) => ({
    byKind: { ...s.byKind, operation: { ...s.byKind.operation, [key]: { v: ++v, data: op } } },
  }))
  forgetLocalOperation(op.id, op.version)
}

/** No operation anywhere. */
export function resetOperations(): void {
  entityStore.setState((s) => ({
    byKind: { ...s.byKind, operation: {}, review_thread: {} },
    members: {},
    fresh: {},
  }))
  useLocalOperationsStore.setState({ ops: {} })
}
