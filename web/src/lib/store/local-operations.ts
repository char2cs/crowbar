import { create } from 'zustand'
import type { Operation } from '@/lib/store/operations'

/** Operations the stream has not delivered: started here and unanswered, or a
 *  daemon reply that beat its frame. The stream's copy replaces each (core/entities). */
export const useLocalOperationsStore = create<{ ops: Record<string, Operation> }>()(() => ({
  ops: {},
}))

/** Drops the local copy of `id` once the stream delivered the daemon's row at
 *  `version`; a newer reply (a dismiss) stays until its own frame. */
export function forgetLocalOperation(id: string, version: number): void {
  useLocalOperationsStore.setState((s) => {
    const held = s.ops[id]
    if (!held || (!held.local && held.version > version)) return s
    const ops = { ...s.ops }
    delete ops[id]
    return { ops }
  })
}

/** At a complete snapshot: a daemon reply the stream does not carry was pruned
 *  by the daemon, so no frame will ever replace it. In-flight local ones stay. */
export function forgetUnstreamedReplies(streamed: (id: string) => boolean): void {
  useLocalOperationsStore.setState((s) => {
    const gone = Object.values(s.ops).filter((op) => !op.local && !streamed(op.id))
    if (gone.length === 0) return s
    const ops = { ...s.ops }
    for (const op of gone) delete ops[op.id]
    return { ops }
  })
}
