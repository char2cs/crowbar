import { readDocument, workspaceRef } from '@/lib/api/documents'
import { toast } from '@/features/window/stores/toast-store'

interface BufferOpener {
  openContent: (
    spec: {
      type: 'editor'
      path: string
      name: string
      content: string
      isPreview?: boolean
      workspaceId?: string
    },
    opts?: { paneId?: string },
  ) => void
}

/**
 * Load a file's content from the daemon and open it in an editor buffer. On
 * failure, surface a toast instead of failing silently (the previous behavior
 * left an uncaught promise and no user feedback). `path` is workspace-relative.
 */
export async function openFileContent(
  wsId: string,
  path: string,
  bufferActions: BufferOpener,
  opts: { preview?: boolean; paneId?: string } = {},
): Promise<void> {
  const name = path.split('/').pop() ?? path
  try {
    const { text } = await readDocument(workspaceRef(wsId, path))
    bufferActions.openContent(
      {
        type: 'editor',
        path,
        name,
        content: text,
        isPreview: opts.preview,
        workspaceId: wsId,
      },
      { paneId: opts.paneId },
    )
  } catch {
    toast.error('Failed to open file', name)
  }
}
