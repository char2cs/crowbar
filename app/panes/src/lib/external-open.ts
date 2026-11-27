import { isDesktopApp, isTauri } from '@/lib/crowbar-bridge'
import { postShellFrame } from '@/sdk/post-shell-frame'

const OPENABLE = /^(?:https?:\/\/|mailto:|tel:)\S/i

export async function openExternalUrl(url: string): Promise<void> {
  if (!OPENABLE.test(url)) throw new Error(`not a web, mail or phone link: ${url}`)
  if (postShellFrame({ kind: 'shell.openExternal', url })) return
  if (isDesktopApp()) {
    const { open } = await import('@tauri-apps/plugin-shell')
    await open(url)
    return
  }
  if (isTauri()) throw new Error(`the phone app cannot open links yet: ${url}`)
  window.open(url, '_blank', 'noopener,noreferrer')
}
