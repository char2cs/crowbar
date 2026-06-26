import { apiFetch } from '@/lib/api'
import { workspaceBase } from '@/lib/workspace-scope-url'
import { toast } from '@/features/window/stores/toast-store'

interface BufferOpener {
  openContent: (spec: {
    type: 'editor'
    path: string
    name: string
    content: string
    isPreview?: boolean
  }) => void
}

interface FileContentPayload {
  content: string
  encoding?: string
}

function decode(payload: FileContentPayload): string {
  if (payload.encoding === 'base64') return atob(payload.content)
  return payload.content
}

/**
 * Load a file's content from the workspace-scoped backend and open it in an
 * editor buffer. On failure, surface a toast instead of failing silently (the
 * previous behavior left an uncaught promise and no user feedback). `path` is
 * workspace-relative.
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
      `${workspaceBase(wsId)}/files/content?path=${encodeURIComponent(path)}`,
    )
    bufferActions.openContent({
      type: 'editor',
      path,
      name,
      content: decode(payload),
      isPreview: opts.preview,
    })
  } catch {
    toast.error('Failed to open file', name)
  }
}
