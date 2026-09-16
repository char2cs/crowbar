import { apiFetch } from '@/lib/api'
import { filesBaseForWorkspace } from '@/lib/workspace-scope-url'
import {
  decodeFileContent,
  type FileContentPayload,
} from '@/features/file-system/utils/file-content-encoding'
import { toast } from '@/features/window/stores/toast-store'

interface BufferOpener {
  openContent: (spec: {
    type: 'editor'
    path: string
    name: string
    content: string
    isPreview?: boolean
    workspaceId?: string
  }) => void
}

/**
 * Load a file's content from the daemon and open it in an editor buffer. On
 * failure, surface a toast instead of failing silently (the previous behavior
 * left an uncaught promise and no user feedback). `path` is workspace-relative
 * — that is the worktree the read resolves to, not the URL it is addressed by
 * (see filesBaseForWorkspace).
 */
export async function openFileContent(
  wsId: string,
  path: string,
  bufferActions: BufferOpener,
  opts: { preview?: boolean } = {},
): Promise<void> {
  const name = path.split('/').pop() ?? path
  try {
    const payload = await apiFetch<FileContentPayload>(
      `${filesBaseForWorkspace(wsId)}/content?path=${encodeURIComponent(path)}`,
    )
    bufferActions.openContent({
      type: 'editor',
      path,
      name,
      content: decodeFileContent(payload),
      isPreview: opts.preview,
      workspaceId: wsId,
    })
  } catch {
    toast.error('Failed to open file', name)
  }
}
