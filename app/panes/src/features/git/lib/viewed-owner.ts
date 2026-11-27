import type { ViewedChange } from '@/sdk/viewed-change'
import type { ViewedListener } from './viewed-listener'

export interface ViewedOwner {
  follow(wsId: string, listener: ViewedListener): () => void
  set(wsId: string, change: ViewedChange): Promise<void>
}

const page: { owner: ViewedOwner | null } = { owner: null }

export function installViewedOwner(owner: ViewedOwner): void {
  page.owner = owner
}

export function viewedOwner(): ViewedOwner {
  if (!page.owner) throw new Error('no viewed owner was installed for this page')
  return page.owner
}
