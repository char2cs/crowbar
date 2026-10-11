import { openWorkspaceLink } from '@/lib/workspace-link'
import { FitAddon } from '@xterm/addon-fit'
import { SearchAddon } from '@xterm/addon-search'
import { SerializeAddon } from '@xterm/addon-serialize'
import { Unicode11Addon } from '@xterm/addon-unicode11'
import { WebLinksAddon } from '@xterm/addon-web-links'
import type { Terminal } from '@xterm/xterm'

export interface TerminalAddons {
  fitAddon: FitAddon
  searchAddon: SearchAddon
  serializeAddon: SerializeAddon
}

export function createTerminalAddons(terminal: Terminal): TerminalAddons {
  const fitAddon = new FitAddon()
  const searchAddon = new SearchAddon()
  const serializeAddon = new SerializeAddon()
  const unicode11Addon = new Unicode11Addon()

  terminal.loadAddon(fitAddon)
  terminal.loadAddon(searchAddon)
  terminal.loadAddon(serializeAddon)
  terminal.loadAddon(unicode11Addon)

  return { fitAddon, searchAddon, serializeAddon }
}

/** Opens a clicked URL in the default browser, with no confirmation step — unless it is a
 *  loopback address printed in a workspace on another host (see openWorkspaceLink). */
export function loadWebLinksAddon(terminal: Terminal, workspaceId: () => string): void {
  const webLinksAddon = new WebLinksAddon((_event: MouseEvent, uri: string) => {
    openWorkspaceLink(uri, workspaceId()).catch((error) => {
      console.error('Failed to open link:', error)
    })
  })
  terminal.loadAddon(webLinksAddon)
}

export function injectLinkStyles(sessionId: string, containerId: string): void {
  const styleId = `terminal-link-style-${sessionId}`
  if (document.getElementById(styleId)) return

  const style = document.createElement('style')
  style.id = styleId
  const accentColor = getComputedStyle(document.documentElement)
    .getPropertyValue('--color-accent')
    .trim()
  // Escaped: an agent pane's id carries a PTY id its daemon chose.
  const scope = `#${CSS.escape(containerId)}`

  style.textContent = `
    ${scope} .xterm-screen a,
    ${scope} .xterm-link,
    ${scope} [style*="text-decoration"] {
      color: ${accentColor} !important;
      text-decoration: underline !important;
      cursor: pointer !important;
    }
    ${scope} .xterm-screen a:hover,
    ${scope} .xterm-link:hover {
      opacity: 0.8 !important;
    }
  `
  document.head.appendChild(style)
}

export function removeLinkStyles(sessionId: string): void {
  const styleId = `terminal-link-style-${sessionId}`
  const style = document.getElementById(styleId)
  if (style) {
    style.remove()
  }
}
