/**
 * Calls `onChange` whenever `node` may have moved or resized in the viewport,
 * without polling. A ResizeObserver cannot see a move that keeps the size, so an
 * IntersectionObserver whose root is shrunk to exactly the node's box fires on
 * any shift; it is rebuilt around the new box after each report.
 */
export function observeRect(node: HTMLElement, onChange: () => void): () => void {
  let layoutWatcher: IntersectionObserver | null = null

  const watchLayout = () => {
    layoutWatcher?.disconnect()
    const box = node.getBoundingClientRect()
    const { clientWidth, clientHeight } = document.documentElement
    const inset = (px: number) => `${-Math.floor(px)}px`
    let first = true
    layoutWatcher = new IntersectionObserver(
      ([entry]) => {
        // The initial delivery only reports the starting state.
        if (first || entry.intersectionRatio === 1) {
          first = false
          return
        }
        report()
      },
      {
        threshold: 1,
        rootMargin: [
          inset(box.top),
          inset(clientWidth - box.left - box.width),
          inset(clientHeight - box.top - box.height),
          inset(box.left),
        ].join(' '),
      },
    )
    layoutWatcher.observe(node)
  }

  const report = () => {
    onChange()
    watchLayout()
  }

  const resizeWatcher = new ResizeObserver(report)
  resizeWatcher.observe(node)
  // A window resize changes the viewport the layout watcher's root is cut from.
  window.addEventListener('resize', report)
  watchLayout()

  return () => {
    resizeWatcher.disconnect()
    layoutWatcher?.disconnect()
    window.removeEventListener('resize', report)
  }
}
