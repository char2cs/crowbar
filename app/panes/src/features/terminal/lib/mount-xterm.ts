import { Terminal, type ITerminalOptions } from '@xterm/xterm'
import { createTerminalAddons, type TerminalAddons } from '../hooks/use-terminal-addons'
import { registerTerminalLinks, type TerminalLinksOptions } from './terminal-links'
import { resolveKeyOverride } from '../utils/terminal-key-overrides'
import { installInputTapeGlobal, observeInputEvents } from '../utils/input-tape'

export interface MountedXterm {
  terminal: Terminal
  addons: TerminalAddons
  dispose: () => void
}

interface MountXtermDeps {
  write: (data: string, origin: string) => void
  links: TerminalLinksOptions
}

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
    const terminal = new Terminal({ ...options, allowProposedApi: true })
    releases.push(() => terminal.dispose())
    const addons = createTerminalAddons(terminal)
    terminal.open(container)
    terminal.attachCustomKeyEventHandler((event) => {
      const override = resolveKeyOverride(event)
      if (override !== null) {
        event.preventDefault()
        deps.write(override, 'modifier-enter-override')
        return false
      }
      if (linuxClipboardKey(event, terminal)) return false
      if (event.ctrlKey && !event.metaKey) return true
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
