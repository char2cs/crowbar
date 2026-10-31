import { create } from 'zustand'
import type { Operation } from '@/lib/store/operations'

/** Operations the stream has not delivered: started here and unanswered, or a
 *  daemon reply that beat its frame. The stream's copy replaces each (core/entities). */
export const useLocalOperationsStore = create<{ ops: Record<string, Operation> }>()(() => ({
  ops: {},
}))

/** Drops the local copy of `id` once the stream delivered the daemon's own. */
export function forgetLocalOperation(id: string): void {
  useLocalOperationsStore.setState((s) => {
    if (!(id in s.ops)) return s
    const ops = { ...s.ops }
    delete ops[id]
    return { ops }
  })
}
