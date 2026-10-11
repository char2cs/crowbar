import { LOCAL_HOST } from '@/features/hosts/types'
import { apiFetch } from '@/lib/api'
import { encodeMultipart, readDesktopFile } from '@/lib/desktop-file-upload'
import { splitHostPath } from '@/lib/host-path'

// The daemon's icons.MaxBytes is the authority on the cap.
const ICON_LIMIT = { bytes: 2 * 1024 * 1024, label: '2 MB icon' }

/** Sets an entity's icon (`url`, its host-qualified `.../icon` route) to image `path` on this
 *  computer: the local daemon reads the path itself; a remote one, which cannot see this disk,
 *  is sent the image's bytes. */
export async function putIconFile(url: string, path: string): Promise<void> {
  if (splitHostPath(url).hostId === LOCAL_HOST.id) {
    await apiFetch(url, {
      method: 'PUT',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ path }),
    })
    return
  }
  const image = await readDesktopFile(path, ICON_LIMIT)
  const { body, contentType } = await encodeMultipart({}, 'icon', image)
  await apiFetch(url, { method: 'PUT', headers: { 'Content-Type': contentType }, body })
}
