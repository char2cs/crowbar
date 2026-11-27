import type { PaneClient } from '@/sdk/pane-client'
import { useZoomStore } from '@/features/window/stores/zoom-store'

export function followChatZoom(client: Pick<PaneClient, 'onChatZoom'>): () => void {
  return client.onChatZoom((percent) => useZoomStore.getState().actions.setZoom(percent / 100))
}
