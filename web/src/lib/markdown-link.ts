// Click handling for links in rendered markdown. In the Tauri WKWebView a bare `<a href>` click
// navigates the whole app view, so every click cancels default navigation; external URLs go to
// the OS browser, and relative/anchor links are left to the caller.
import { createContext, useContext } from 'react'
import { openWorkspaceLink } from '@/lib/workspace-link'

const EXTERNAL_SCHEME = /^(?:https?|mailto|tel):/i

/** A URL for the OS browser/mail/dialer, not the webview; protocol-relative `//host/…` counts. */
export function isExternalUrl(url: string): boolean {
  return url.startsWith('//') || EXTERNAL_SCHEME.test(url)
}

/** The workspace whose content a subtree renders — its links' host. Null: no workspace's. */
export const LinkWorkspaceContext = createContext<string | null>(null)

export function useLinkWorkspace(): string | null {
  return useContext(LinkWorkspaceContext)
}

// Handles a click on a rendered markdown anchor in `wsId`'s content: always cancels webview
// navigation and opens external hrefs via openWorkspaceLink. No href: nothing is cancelled.
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
