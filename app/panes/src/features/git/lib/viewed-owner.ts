import type { ViewedChange } from '@/sdk/viewed-change'
import type { ViewedListener } from './viewed-listener'

/** Who keeps the viewed maps. A page has exactly one, installed once at its entry point. */
export interface ViewedOwner {
  /** Calls `listener` with the workspace's map once it is known and on every later change;
   *  returns the stop. */
  follow(wsId: string, listener: ViewedListener): () => void
  /** Marks or unmarks a batch; the result reaches the page only through `follow`. */
  set(wsId: string, change: ViewedChange): Promise<void>
}

const page: { owner: ViewedOwner | null } = { owner: null }

/** Makes `owner` the page's viewed owner. */
export function installViewedOwner(owner: ViewedOwner): void {
  page.owner = owner
}

/** The page's viewed owner; throws when the entry point installed none. */
export function viewedOwner(): ViewedOwner {
  if (!page.owner) throw new Error('no viewed owner was installed for this page')
  return page.owner
}
