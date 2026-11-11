import { Terminal, type ITerminalOptions } from '@xterm/xterm'
import { createTerminalAddons, type TerminalAddons } from '../hooks/use-terminal-addons'
import { registerTerminalLinks, type TerminalLinksOptions } from './terminal-links'
import { resolveKeyOverride } from '../utils/terminal-key-overrides'
import { installInputTapeGlobal, observeInputEvents } from '../utils/input-tape'

export interface MountedXterm {
  terminal: Terminal
  addons: TerminalAddons
  /** Releases everything the mount attached, then the terminal itself. */
  dispose: () => void
}

interface MountXtermDeps {
  /** Sends input the terminal itself produces (key overrides, drops, IME replacements). */
  write: (data: string, origin: string) => void
  links: TerminalLinksOptions
}

/** Returns the one dispose for everything a view hangs on its xterm; a throw part-way releases
 *  what was already built, so a failed mount leaks nothing. */
export function mountXterm(
  container: HTMLElement,
  options: ITerminalOptions,
  deps: MountXtermDeps,
): MountedXterm {
  const releases: Array<() => void> = []
  const dispose = () => {
    for (const release of releases.splice(0).reverse()) release()
  }
  try {
    // The unicode11 addon is a proposed API.
    const terminal = new Terminal({ ...options, allowProposedApi: true })
    releases.push(() => terminal.dispose())
    const addons = createTerminalAddons(terminal)
    terminal.open(container)
    terminal.attachCustomKeyEventHandler((event) => {
      // The ONLY manual key override (Shift/Alt+Enter): emit the CSI-u sequence
      // and return false to suppress xterm's default CR, so it is sent once.
      const override = resolveKeyOverride(event)
      if (override !== null) {
        event.preventDefault()
        deps.write(override, 'modifier-enter-override')
        return false
      }
      // Linux terminals copy and paste on Ctrl+Shift+C/V; the rest of Ctrl reaches the
      // program. A paste is left to the browser, whose paste event xterm takes.
      if (linuxClipboardKey(event, terminal)) return false
      // Ctrl combos (without Cmd) → xterm handles them (Ctrl+U, Ctrl+C, …).
      if (event.ctrlKey && !event.metaKey) return true
      // Cmd combos are app/OS shortcuts (copy, paste, select-all, search).
      return !event.metaKey
    })
    if (terminal.textarea) releases.push(watchTextarea(terminal.textarea, deps.write))
    const links = registerTerminalLinks(terminal, deps.links)
    releases.push(() => links.dispose())
    terminal.unicode.activeVersion = '11'
    return { terminal, addons, dispose }
  } catch (error) {
    dispose()
    throw error
  }
}

function linuxClipboardKey(event: KeyboardEvent, terminal: Terminal): boolean {
  if (!event.ctrlKey || !event.shiftKey || event.metaKey || event.altKey) return false
  if (event.code === 'KeyV') return true
  if (event.code !== 'KeyC') return false
  event.preventDefault()
  if (event.type === 'keydown' && terminal.hasSelection()) document.execCommand('copy')
  return true
}

function watchTextarea(textarea: HTMLTextAreaElement, write: MountXtermDeps['write']): () => void {
  textarea.spellcheck = false
  // Observational only: records the raw key/input/composition events this
  // textarea receives, so a duplicated or missing character can be traced.
  installInputTapeGlobal()
  const stopObserving = observeInputEvents(textarea)
  const onBeforeInput = (event: InputEvent) => {
    if (event.inputType !== 'insertReplacementText' && event.inputType !== 'insertFromDrop') return
    const text = event.dataTransfer?.getData('text/plain') ?? event.data
    if (!text) return
    event.preventDefault()
    write(text, `beforeinput:${event.inputType}`)
  }
  textarea.addEventListener('beforeinput', onBeforeInput)
  return () => {
    textarea.removeEventListener('beforeinput', onBeforeInput)
    stopObserving()
  }
}
