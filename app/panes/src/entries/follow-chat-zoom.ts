import type { PaneClient } from '@/sdk/pane-client'
import { useZoomStore } from '@/features/window/stores/zoom-store'

/** Shows the chat at the shell window's chat zoom, which the shell keeps for every chat as develop's one zoom store does; returns the stop. */
export function followChatZoom(client: Pick<PaneClient, 'onChatZoom'>): () => void {
  return client.onChatZoom((percent) => useZoomStore.getState().actions.setZoom(percent / 100))
}
