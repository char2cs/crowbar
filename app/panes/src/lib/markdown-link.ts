import { createContext, useContext } from 'react'
import { openWorkspaceLink } from '@/lib/workspace-link'

const EXTERNAL_SCHEME = /^(?:https?|mailto|tel):/i

function isExternalUrl(url: string): boolean {
  return url.startsWith('//') || EXTERNAL_SCHEME.test(url)
}

export const LinkWorkspaceContext = createContext<string | null>(null)

export function useLinkWorkspace(): string | null {
  return useContext(LinkWorkspaceContext)
}

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
