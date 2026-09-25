import { openExternalUrl } from '@/lib/external-open'
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

export function loadWebLinksAddon(terminal: Terminal): void {
  // No confirmation step by design: clicking a URL goes straight to the
  // default browser. openExternalUrl (not window.open): window.open is a
  // silent no-op in the Tauri WKWebView.
  const webLinksAddon = new WebLinksAddon((_event: MouseEvent, uri: string) => {
    openExternalUrl(uri).catch((error) => {
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

  style.textContent = `
    #${containerId} .xterm-screen a,
    #${containerId} .xterm-link,
    #${containerId} [style*="text-decoration"] {
      color: ${accentColor} !important;
      text-decoration: underline !important;
      cursor: pointer !important;
    }
    #${containerId} .xterm-screen a:hover,
    #${containerId} .xterm-link:hover {
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
