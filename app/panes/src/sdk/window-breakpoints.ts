import type { PaneClient } from './pane-client'

/** Names the breakpoints the shell's window clears in `root`'s `data-window` attribute, which the stylesheet's `sm:`…`xl:` variants ask in place of the pane's own width. Returns the stop. */
export function followBreakpoints(
  client: Pick<PaneClient, 'onBreakpoints'>,
  root: HTMLElement = document.documentElement,
): () => void {
  const stop = client.onBreakpoints((cleared) =>
    root.setAttribute('data-window', cleared.join(' ')),
  )
  return () => {
    stop()
    root.removeAttribute('data-window')
  }
}
