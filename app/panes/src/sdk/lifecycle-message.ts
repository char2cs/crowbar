import type { PaneParams } from './pane-params'

type LifecycleEvent = 'mount' | 'activate' | 'deactivate' | 'park' | 'destroy'

/** A lifecycle event of this page's pane, with the ids and the state it saved last (`null` before it saved any). */
export interface LifecycleMessage {
  paneId: string
  event: LifecycleEvent
  params: PaneParams
  state: unknown
}
