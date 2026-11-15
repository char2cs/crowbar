import type { PaneClient } from '@/sdk/pane-client'

/** What a pane page needs once the shell's runtime is up. */
export interface BootedPane {
  client: PaneClient
  container: HTMLElement
}
