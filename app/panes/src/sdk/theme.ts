import type { PaneClient } from './pane-client'

export function followTheme(
  client: Pick<PaneClient, 'onTheme'>,
  root: HTMLElement = document.documentElement,
): () => void {
  let published: string[] = []
  const clear = () => published.forEach((name) => root.style.removeProperty(`--${name}`))
  const stop = client.onTheme((tokens) => {
    clear()
    published = Object.keys(tokens)
    Object.entries(tokens).forEach(([name, value]) => root.style.setProperty(`--${name}`, value))
  })
  return () => {
    stop()
    clear()
  }
}
