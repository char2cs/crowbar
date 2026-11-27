import type { PaneClient } from './pane-client'

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
