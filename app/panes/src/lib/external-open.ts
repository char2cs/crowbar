// The one entry point for opening a URL in the default browser: the shell opens it, the phone
// cannot yet (a link in its one webview would replace the app), a plain browser uses window.open.
import { isTauri } from '@/lib/crowbar-bridge'
import { postShellFrame } from '@/sdk/post-shell-frame'

// What the shell's opener admits: a URL's text may come from another host,
// and no other scheme (file:, javascript:, a custom app scheme) may leave the app from it.
const OPENABLE = /^(?:https?:\/\/|mailto:|tel:)\S/i

export async function openExternalUrl(url: string): Promise<void> {
  if (!OPENABLE.test(url)) throw new Error(`not a web, mail or phone link: ${url}`)
  if (postShellFrame({ kind: 'shell.openExternal', url })) return
  // The phone app ships no shell plugin; a native opener has to come from its Swift side.
  if (isTauri()) throw new Error(`the phone app cannot open links yet: ${url}`)
  window.open(url, '_blank', 'noopener,noreferrer')
}
