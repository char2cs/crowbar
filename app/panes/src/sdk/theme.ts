import type { PaneClient } from './pane-client'

/** Publishes the shell's design tokens as CSS custom properties (`color-bg` becomes `--color-bg`) on `root` and keeps them current; a token the shell stopped sending is removed. Returns the stop. */
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
