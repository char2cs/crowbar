import type { PaneClient } from '@/sdk/pane-client'
import type { ViewedChange } from '@/sdk/viewed-change'
import type { ViewedListener } from './viewed-listener'
import type { ViewedOwner } from './viewed-owner'

export class ShellViewedOwner implements ViewedOwner {
  private readonly heard = new Map<string, Record<string, string>>()
  private readonly listeners = new Map<string, Set<ViewedListener>>()

  constructor(private readonly client: PaneClient) {
    client.onReviewViewed(({ workspace, viewed }) => {
      this.heard.set(workspace, viewed)
      this.listeners.get(workspace)?.forEach((listener) => listener(viewed))
    })
  }

  follow(wsId: string, listener: ViewedListener): () => void {
    const listeners = this.listeners.get(wsId) ?? new Set<ViewedListener>()
    this.listeners.set(wsId, listeners)
    listeners.add(listener)
    const known = this.heard.get(wsId)
    if (known) listener(known)
    return () => void listeners.delete(listener)
  }

  set(_wsId: string, change: ViewedChange): Promise<void> {
    this.client.setReviewViewed(change)
    return Promise.resolve()
  }
}
