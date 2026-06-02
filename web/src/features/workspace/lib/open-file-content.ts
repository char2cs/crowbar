import { apiFetch } from '@/lib/api'
import { toast } from '@/components/ui/toast'

interface BufferOpener {
  openContent: (spec: {
    type: 'editor'
    path: string
    name: string
    content: string
    isPreview?: boolean
  }) => void
}

/**
 * Load a file's content and open it in an editor buffer. On failure, surface a
 * toast instead of failing silently (the previous behavior left an uncaught
 * promise and no user feedback).
 */
export async function openFileContent(
  path: string,
  bufferActions: BufferOpener,
  opts: { preview?: boolean } = {},
): Promise<void> {
  const name = path.split('/').pop() ?? path
  try {
    const content = await apiFetch<string>(`/api/v0/fs/file?path=${encodeURIComponent(path)}`)
    bufferActions.openContent({ type: 'editor', path, name, content, isPreview: opts.preview })
  } catch {
    toast.error('Failed to open file', { description: name })
  }
}
