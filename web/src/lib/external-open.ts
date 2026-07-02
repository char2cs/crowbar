// Single approved entry point for opening a URL in the user's default
// browser. Components import from here instead of `@tauri-apps/plugin-shell`
// directly so the eslint `no-restricted-imports` bridge-only rule has one
// place to allow.
//
// In the Tauri app `window.open(url, '_blank')` is a silent no-op — the
// WKWebView has no new-window handler — so the shell plugin's opener is the
// only path to the default browser. Browser dev keeps window.open.
import { isTauri } from '@/lib/crowbar-bridge'

export async function openExternalUrl(url: string): Promise<void> {
  if (isTauri()) {
    const { open } = await import('@tauri-apps/plugin-shell')
    await open(url)
    return
  }
  window.open(url, '_blank', 'noopener,noreferrer')
}
