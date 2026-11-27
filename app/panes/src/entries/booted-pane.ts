import type { PaneClient } from '@/sdk/pane-client'

export interface BootedPane {
  client: PaneClient
  container: HTMLElement
}
