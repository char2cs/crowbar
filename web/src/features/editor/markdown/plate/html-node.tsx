'use client'

import { useEffect, useRef } from 'react'
import DOMPurify from 'dompurify'
import { createPlatePlugin, PlateElement, type PlateElementProps } from 'platejs/react'
import { handleMarkdownAnchorClick, useLinkWorkspace } from '@/lib/markdown-link'
import { loadLocalImage, useMarkdownAsset } from './markdown-asset'

/** A void block carrying a raw HTML string (a README's `<div align="center">` header): rendered
 *  sanitized, saved byte-for-byte via markdown-html-rules.ts. Not editable in place; change it in
 *  Source view, as with YAML frontmatter. */
export interface HtmlElementNode {
  type: 'html'
  html: string
  children: [{ text: '' }]
}

// The HTML comes from a file or an agent's message, maybe on another host: beyond scripts, it
// may not restyle the app around it (style, class), take typing (forms) or carry inline SVG —
// what README hosts drop too.
const HTML_BLOCK_POLICY = {
  USE_PROFILES: { html: true },
  FORBID_TAGS: ['style', 'form', 'input', 'button', 'textarea', 'select', 'option', 'map', 'area'],
  FORBID_ATTR: ['style', 'class', 'action', 'formaction'],
}

export function HtmlElement(props: PlateElementProps) {
  const raw = (props.element as unknown as { html?: string }).html ?? ''
  // Display-only: serialization keeps the untouched `html` string, so this cannot corrupt the file.
  const clean = DOMPurify.sanitize(raw, HTML_BLOCK_POLICY)

  const asset = useMarkdownAsset()
  const linkWorkspace = useLinkWorkspace()
  const ref = useRef<HTMLDivElement>(null)

  // Local `<img src="./logo.png">` refs don't resolve in the webview (its base URL is the dev
  // server), so after paint each local src is rewritten to a `data:` URL read from the workspace.
  // Remote and data srcs are left alone; late reads are cancelled.
  useEffect(() => {
    const el = ref.current
    if (!el || !asset) return
    const controller = new AbortController()
    for (const img of Array.from(el.querySelectorAll('img'))) {
      const src = img.getAttribute('src')
      if (!src) continue
      void loadLocalImage(asset, src, controller.signal).then((url) => {
        if (url && !controller.signal.aborted) img.src = url
      })
    }
    return () => controller.abort()
  }, [clean, asset])

  // A native delegated listener, not onClick: the controls are `<a>`s in HTML we don't own (Enter
  // on one dispatches a click), so the container takes no role. Every anchor click is cancelled and
  // routed to the OS browser, since in the Tauri WKWebView it would navigate the whole app.
  useEffect(() => {
    const el = ref.current
    if (!el) return
    const onClick = (e: MouseEvent) => {
      const anchor = (e.target as HTMLElement | null)?.closest('a')
      if (anchor) handleMarkdownAnchorClick(e, anchor.getAttribute('href'), linkWorkspace)
    }
    el.addEventListener('click', onClick)
    return () => el.removeEventListener('click', onClick)
  }, [linkWorkspace])

  return (
    <PlateElement {...props}>
      <div
        ref={ref}
        className="markdown-html-block"
        contentEditable={false}
        // react-doctor-disable-next-line dangerous-html-sink -- `clean` is DOMPurify.sanitize() output of an opaque HTML string the file already contains; display-only, never re-serialized.
        dangerouslySetInnerHTML={{ __html: clean }}
      />
      {props.children}
    </PlateElement>
  )
}

/** Void `html` element plugin — registers the node type the markdown rules emit. */
export const HtmlKit = [
  createPlatePlugin({
    key: 'html',
    node: {
      isElement: true,
      isVoid: true,
      component: HtmlElement,
    },
  }),
]
