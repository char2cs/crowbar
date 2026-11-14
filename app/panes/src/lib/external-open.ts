// The one entry point for opening a URL in the default browser, so the bridge-only
// `no-restricted-imports` rule has one place to allow. In Tauri, `window.open(url, '_blank')` is a
// silent no-op (WKWebView has no new-window handler); browser dev keeps window.open.
import { isDesktopApp, isTauri } from '@/lib/crowbar-bridge'

// What the desktop's own shell-open scope admits: a URL's text may come from another host,
// and no other scheme (file:, javascript:, a custom app scheme) may leave the app from it.
const OPENABLE = /^(?:https?:\/\/|mailto:|tel:)\S/i

export async function openExternalUrl(url: string): Promise<void> {
  if (!OPENABLE.test(url)) throw new Error(`not a web, mail or phone link: ${url}`)
  if (isDesktopApp()) {
    const { open } = await import('@tauri-apps/plugin-shell')
    await open(url)
    return
  }
  // The phone app ships no shell plugin; a native opener has to come from its Swift side.
  if (isTauri()) throw new Error(`the phone app cannot open links yet: ${url}`)
  window.open(url, '_blank', 'noopener,noreferrer')
}
