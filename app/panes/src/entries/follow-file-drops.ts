import { deliverShellFileDrop } from '@/features/file-system/lib/tauri-file-drop'
import type { PaneClient } from '@/sdk/pane-client'

/** Gives each file drop on this page to the element under it, as develop's pages take Tauri's drop
 *  event; a drop no element takes opens its files as editor tabs from this pane, in drop order, as
 *  develop's pane does. Returns the stop. */
export function followFileDrops(client: Pick<PaneClient, 'onFilesDropped' | 'open'>): () => void {
  return client.onFilesDropped(({ paths, x, y }) => {
    if (deliverShellFileDrop({ x, y }, paths)) return
    for (const file of paths) client.open('core.editor', {}, { file, mode: 'kept' })
  })
}
