// Tauri plugins replaced with browser equivalents
const ask = async (message: string, _options?: Record<string, unknown>): Promise<boolean> => window.confirm(message)
const open = async (url: string) => { window.open(url, '_blank') }
import { FitAddon } from "@xterm/addon-fit";
import { SearchAddon } from "@xterm/addon-search";
import { SerializeAddon } from "@xterm/addon-serialize";
import { Unicode11Addon } from "@xterm/addon-unicode11";
import { WebLinksAddon } from "@xterm/addon-web-links";
import { WebglAddon } from "@xterm/addon-webgl";
import type { Terminal } from "@xterm/xterm";

export interface TerminalAddons {
  fitAddon: FitAddon;
  searchAddon: SearchAddon;
  serializeAddon: SerializeAddon;
  webglAddon: WebglAddon | null;
}

export interface CreateTerminalAddonsOptions {
  /** Skip WebGL addon - use canvas renderer instead (better for some fonts like Nerd Fonts) */
  skipWebGL?: boolean;
}

export function createTerminalAddons(
  terminal: Terminal,
  options: CreateTerminalAddonsOptions = {},
): TerminalAddons {
  const fitAddon = new FitAddon();
  const searchAddon = new SearchAddon();
  const serializeAddon = new SerializeAddon();
  const unicode11Addon = new Unicode11Addon();

  terminal.loadAddon(fitAddon);
  terminal.loadAddon(searchAddon);
  terminal.loadAddon(serializeAddon);
  terminal.loadAddon(unicode11Addon);

  let webglAddon: WebglAddon | null = null;

  if (!options.skipWebGL) {
    webglAddon = new WebglAddon();
    webglAddon.onContextLoss(() => {
      webglAddon?.dispose();
    });
    terminal.loadAddon(webglAddon);
  }

  return { fitAddon, searchAddon, serializeAddon, webglAddon };
}

export function loadWebLinksAddon(terminal: Terminal): void {
  const webLinksAddon = new WebLinksAddon(async (_event: MouseEvent, uri: string) => {
    try {
      const confirmed = await ask(`Do you want to open this link in your browser?\n\n${uri}`, {
        title: "Open External Link",
        kind: "warning",
        okLabel: "Open",
        cancelLabel: "Cancel",
      });

      if (confirmed) {
        await open(uri);
      }
    } catch (error) {
      console.error("Failed to open link:", error);
    }
  });
  terminal.loadAddon(webLinksAddon);
}

export function injectLinkStyles(sessionId: string, containerId: string): void {
  const styleId = `terminal-link-style-${sessionId}`;
  if (document.getElementById(styleId)) return;

  const style = document.createElement("style");
  style.id = styleId;
  const accentColor = getComputedStyle(document.documentElement)
    .getPropertyValue("--color-accent")
    .trim();

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
  `;
  document.head.appendChild(style);
}

export function removeLinkStyles(sessionId: string): void {
  const styleId = `terminal-link-style-${sessionId}`;
  const style = document.getElementById(styleId);
  if (style) {
    style.remove();
  }
}
