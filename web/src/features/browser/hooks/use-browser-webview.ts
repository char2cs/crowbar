import { useLayoutEffect, type RefObject } from 'react'
import { report } from '@/lib/errors/report'
import { browserApi, type BrowserRect } from '@/lib/browser-bridge'
import { observeRect } from '@/features/browser/lib/observe-rect'
import { ensureWebview } from '@/features/browser/lib/ensure-webview'
import { webviewCreated } from '@/features/browser/lib/webview-registry'

function rectOf(node: HTMLElement): BrowserRect {
  const { x, y, width, height } = node.getBoundingClientRect()
  return { x, y, width, height }
}

interface Options {
  bufferId: string
  /** The tab has somewhere to go; with none there is nothing to create. */
  hasUrl: boolean
  visible: boolean
  placeholderRef: RefObject<HTMLElement | null>
}

/** Keeps a native webview laid over `placeholderRef` while `visible`, parked off screen
 *  otherwise. A tab switch or pane move unmounts this, which only parks the webview; the
 *  buffer's release closes it (see disposeBuffers). */
export function useBrowserWebview({ bufferId, hasUrl, visible, placeholderRef }: Options) {
  // The webview is made the first time the tab is shown unless the agent got
  // there first: the registry is the one record of whether Rust has it.
  useLayoutEffect(() => {
    const node = placeholderRef.current
    if (!hasUrl || !node) return
    const existing = webviewCreated(bufferId)
    if (!visible) {
      void existing
        ?.then(() => browserApi.park(bufferId))
        .catch((err: unknown) => report(err, { action: 'Hide the page' }))
      return
    }
    const ready = existing ?? ensureWebview(bufferId, rectOf(node), false)
    if (!ready) return
    if (existing) {
      void existing
        .then(() => browserApi.unpark(bufferId, rectOf(node)))
        .catch((err: unknown) => report(err, { action: 'Show the page' }))
    }
    return observeRect(node, () =>
      ready
        .then(() => browserApi.setBounds(bufferId, rectOf(node)))
        .catch((err: unknown) => report(err, { action: 'Resize the page' })),
    )
  }, [bufferId, hasUrl, visible, placeholderRef])

  // A native view outlives its pane's DOM; whoever unmounts must take it off screen.
  useLayoutEffect(() => {
    if (!hasUrl) return
    return () => {
      const created = webviewCreated(bufferId)
      if (!created) return
      void created
        .then(() => browserApi.park(bufferId))
        .catch((err: unknown) => report(err, { action: 'Hide the page' }))
    }
  }, [bufferId, hasUrl])
}
