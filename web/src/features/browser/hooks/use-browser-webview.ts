import { useLayoutEffect, type RefObject } from 'react'
import { bestEffort } from '@/lib/best-effort'
import { windowPaneStore } from '@/features/panes/stores/window-pane-store'
import { browserApi, type BrowserRect } from '@/lib/browser-bridge'
import { useSettingsStore } from '@/features/settings/store'
import { observeRect } from '@/features/browser/lib/observe-rect'
import {
  registerWebview,
  setWebviewVisible,
  webviewCreated,
} from '@/features/browser/lib/webview-registry'

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

/**
 * Keeps a native webview laid over `placeholderRef`. Teardown is not here: a
 * tab switch or a pane move unmounts this, which only hides the webview — the
 * buffer's release closes it (see disposeBuffers).
 */
export function useBrowserWebview({ bufferId, hasUrl, visible, placeholderRef }: Options) {
  // The webview is made the first time the tab is shown, never at restore: the
  // registry is the one record of whether Rust has it.
  useLayoutEffect(() => {
    const node = placeholderRef.current
    if (!hasUrl || !node) return
    let created = webviewCreated(bufferId)
    if (!created) {
      if (!visible) return
      const buffer = windowPaneStore.getState().bufferActions.getBufferById(bufferId)
      if (buffer?.type !== 'browser') return
      const zoom = useSettingsStore.getState().settings.browserZoom / 100
      created = browserApi.open(bufferId, buffer.url, rectOf(node), zoom)
      registerWebview(bufferId, created)
      bestEffort(created, 'open browser webview')
    }
    const ready = created
    return observeRect(node, () =>
      bestEffort(
        ready.then(() => browserApi.setBounds(bufferId, rectOf(node))),
        'move browser webview',
      ),
    )
  }, [bufferId, hasUrl, visible, placeholderRef])

  // Layout effects run in the commit, before paint: the hide for an overlay that
  // mounts in this commit is issued before the frame that would show it.
  useLayoutEffect(() => {
    const created = webviewCreated(bufferId)
    if (!hasUrl || !created) return
    bestEffort(
      created
        .then(() => browserApi.setVisible(bufferId, visible))
        .then(() => setWebviewVisible(bufferId, visible)),
      'toggle browser webview',
    )
  }, [bufferId, hasUrl, visible])

  // A native view outlives its pane's DOM; whoever unmounts must take it off screen.
  useLayoutEffect(() => {
    if (!hasUrl) return
    return () => {
      const created = webviewCreated(bufferId)
      if (!created) return
      bestEffort(
        created
          .then(() => browserApi.setVisible(bufferId, false))
          .then(() => setWebviewVisible(bufferId, false)),
        'hide browser webview',
      )
    }
  }, [bufferId, hasUrl])
}
