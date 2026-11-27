import {
  loadBranchReviewViewed,
  saveBranchReviewViewed,
} from '@/lib/persistence/branch-review-viewed'
import type { ViewedChange } from '@/sdk/viewed-change'
import type { ViewedListener } from './viewed-listener'
import type { ViewedOwner } from './viewed-owner'

export class IdbViewedOwner implements ViewedOwner {
  private readonly listeners = new Map<string, Set<ViewedListener>>()
  private writes: Promise<void> = Promise.resolve()

  follow(wsId: string, listener: ViewedListener): () => void {
    const listeners = this.listeners.get(wsId) ?? new Set<ViewedListener>()
    this.listeners.set(wsId, listeners)
    listeners.add(listener)
    void this.writes
      .then(() => this.read(wsId))
      .then((viewed) => {
        if (listeners.has(listener)) listener(viewed)
      })
    return () => void listeners.delete(listener)
  }

  set(wsId: string, change: ViewedChange): Promise<void> {
    this.writes = this.writes.then(() => this.write(wsId, change))
    return this.writes
  }

  private async read(wsId: string): Promise<Record<string, string>> {
    return (await loadBranchReviewViewed(wsId)) ?? {}
  }

  private async write(wsId: string, change: ViewedChange): Promise<void> {
    const next = { ...(await this.read(wsId)) }
    for (const { path, signature } of change.files) {
      delete next[path]
      if (change.viewed) next[path] = signature
    }
    await saveBranchReviewViewed(wsId, next)
    this.listeners.get(wsId)?.forEach((listener) => listener(next))
  }
}
