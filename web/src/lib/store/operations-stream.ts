import { wsManager } from '@/lib/ws/manager'
import { applyOperationFrame, type Operation } from '@/lib/store/operations'

function isOperation(frame: unknown): frame is Operation {
  const op = frame as Partial<Operation> | null
  return typeof op?.id === 'string' && typeof op.version === 'number'
}

/** Keeps the operations store fed from `/v0/operations`; returns the stop. */
export function startOperationsStream(): () => void {
  return wsManager.subscribe('/v0/operations', (frame) => {
    if (isOperation(frame)) applyOperationFrame(frame)
  })
}
