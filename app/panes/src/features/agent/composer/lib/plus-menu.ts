import type { KeyboardEvent } from 'react'
import { IS_MAC } from '@/utils/platform'

export const ATTACH_SHORTCUT_LABEL = IS_MAC ? '⌘U' : 'Ctrl+U'
export const PASTE_SHORTCUT_LABEL = IS_MAC ? '⌘V' : 'Ctrl+V'

/** The chord the plus menu's "Attach files…" row advertises. */
export function isAttachShortcut(
  event: Pick<KeyboardEvent, 'key' | 'metaKey' | 'ctrlKey' | 'shiftKey' | 'altKey'>,
): boolean {
  const modifier = IS_MAC ? event.metaKey && !event.ctrlKey : event.ctrlKey && !event.metaKey
  return modifier && !event.shiftKey && !event.altKey && event.key.toLowerCase() === 'u'
}

/** The images on the system clipboard, as files named for their type. Empty when it holds none. */
export async function readClipboardImages(): Promise<File[]> {
  const items = await navigator.clipboard.read()
  const files = await Promise.all(
    items.map(async (item) => {
      const type = item.types.find((candidate) => candidate.startsWith('image/'))
      if (!type) return null
      const blob = await item.getType(type)
      return new File([blob], `pasted-image.${type.slice('image/'.length)}`, { type })
    }),
  )
  return files.filter((file): file is File => file !== null)
}
