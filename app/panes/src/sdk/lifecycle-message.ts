import type { PaneParams } from './pane-params'

type LifecycleEvent = 'mount' | 'activate' | 'deactivate' | 'park' | 'destroy'

export interface LifecycleMessage {
  paneId: string
  event: LifecycleEvent
  params: PaneParams
  state: unknown
}
