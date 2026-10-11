// Click handling for links rendered inside markdown (the Plate editor's inline
// `[text](url)` links and the raw-HTML README blocks). A bare `<a href>` click
// inside the Tauri WKWebView navigates the WHOLE app view to the href — there
// is no separate browser chrome to catch it — so every such click must cancel
// its default navigation. External URLs are then handed to the OS default
// browser via the shell opener; relative/anchor links simply stop hijacking the
// webview (the caller may resolve them separately).
import { createContext, useContext } from 'react'
import { openWorkspaceLink } from '@/lib/workspace-link'

const EXTERNAL_SCHEME = /^(?:https?|mailto|tel):/i

/**
 * True for a URL that belongs in the OS default browser / mail / dialer rather
 * than the in-app webview. Protocol-relative `//host/…` counts as external too.
 */
export function isExternalUrl(url: string): boolean {
  return url.startsWith('//') || EXTERNAL_SCHEME.test(url)
}

/** The workspace whose content a subtree renders — its links' host. Null: no workspace's. */
export const LinkWorkspaceContext = createContext<string | null>(null)

export function useLinkWorkspace(): string | null {
  return useContext(LinkWorkspaceContext)
}

/**
 * Handle a click on a rendered markdown anchor in workspace `wsId`'s content. Always cancels
 * the default webview navigation (see module note); external hrefs open through
 * openWorkspaceLink. A missing href is a no-op — nothing is cancelled.
 */
export function handleMarkdownAnchorClick(
  event: Pick<Event, 'preventDefault' | 'stopPropagation'>,
  href: string | null | undefined,
  wsId: string | null,
): void {
  if (!href) return
  event.preventDefault()
  event.stopPropagation()
  if (isExternalUrl(href)) {
    openWorkspaceLink(href, wsId).catch((error: unknown) => {
      console.error('Failed to open link:', error)
    })
  }
}
