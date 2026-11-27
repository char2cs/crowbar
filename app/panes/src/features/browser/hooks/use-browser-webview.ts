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

const after = (created: Promise<unknown>, step: () => Promise<unknown>, action: string) =>
  void created.then(step).catch((err: unknown) => report(err, { action }))

interface Options {
  bufferId: string
  hasUrl: boolean
  visible: boolean
  placeholderRef: RefObject<HTMLElement | null>
}

export function useBrowserWebview({ bufferId, hasUrl, visible, placeholderRef }: Options) {
  useLayoutEffect(() => {
    const node = placeholderRef.current
    if (!hasUrl || !node) return
    const existing = webviewCreated(bufferId)
    if (!visible) {
      if (existing) after(existing, () => browserApi.park(bufferId), 'Hide the page')
      return
    }
    const ready = existing ?? ensureWebview(bufferId, rectOf(node))
    if (!ready) return
    if (existing) after(existing, () => browserApi.unpark(bufferId, rectOf(node)), 'Show the page')
    const resize = () => browserApi.setBounds(bufferId, rectOf(node))
    return observeRect(node, () => after(ready, resize, 'Resize the page'))
  }, [bufferId, hasUrl, visible, placeholderRef])

  useLayoutEffect(() => {
    if (!hasUrl) return
    return () => {
      const created = webviewCreated(bufferId)
      if (created) after(created, () => browserApi.park(bufferId), 'Hide the page')
    }
  }, [bufferId, hasUrl])
}
