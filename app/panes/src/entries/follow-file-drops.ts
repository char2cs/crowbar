import { deliverShellFileDrop } from '@/features/file-system/lib/tauri-file-drop'
import type { PaneClient } from '@/sdk/pane-client'

export function followFileDrops(client: Pick<PaneClient, 'onFilesDropped' | 'open'>): () => void {
  return client.onFilesDropped(({ paths, x, y }) => {
    if (deliverShellFileDrop({ x, y }, paths)) return
    for (const file of paths) client.open('core.editor', {}, { file, mode: 'kept' })
  })
}
