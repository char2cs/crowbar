// Click handling for links rendered inside markdown (the Plate editor's inline
// `[text](url)` links and the raw-HTML README blocks). A bare `<a href>` click
// inside the Tauri WKWebView navigates the WHOLE app view to the href — there
// is no separate browser chrome to catch it — so every such click must cancel
// its default navigation. External URLs are then handed to the OS default
// browser via the shell opener; relative/anchor links simply stop hijacking the
// webview (the caller may resolve them separately).
import { openExternalUrl } from '@/lib/external-open'

const EXTERNAL_SCHEME = /^(?:https?|mailto|tel):/i

/**
 * True for a URL that belongs in the OS default browser / mail / dialer rather
 * than the in-app webview. Protocol-relative `//host/…` counts as external too.
 */
export function isExternalUrl(url: string): boolean {
  return url.startsWith('//') || EXTERNAL_SCHEME.test(url)
}

/**
 * Handle a click on a rendered markdown anchor. Always cancels the default
 * webview navigation (see module note); external hrefs are opened in the OS
 * default browser. A missing href is a no-op — nothing is cancelled.
 */
export function handleMarkdownAnchorClick(
  event: Pick<Event, 'preventDefault' | 'stopPropagation'>,
  href: string | null | undefined,
): void {
  if (!href) return
  event.preventDefault()
  event.stopPropagation()
  if (isExternalUrl(href)) void openExternalUrl(href)
}
