import { LOCAL_HOST } from '@/features/hosts/types'
import { apiFetch } from '@/lib/api/fetch'
import { encodeMultipart, readDesktopFile } from '@/lib/desktop-file-upload'
import { splitHostPath } from '@/lib/host-path'

const ICON_LIMIT = { bytes: 2 * 1024 * 1024, label: '2 MB icon' }

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
