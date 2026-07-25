'use client'

import { useEffect, useRef } from 'react'
import DOMPurify from 'dompurify'
import { createPlatePlugin, PlateElement, type PlateElementProps } from 'platejs/react'
import { handleMarkdownAnchorClick } from '@/lib/markdown-link'
import { loadLocalImage, useMarkdownAsset } from './markdown-asset'

/**
 * A void block that carries a raw HTML string verbatim. READMEs routinely open
 * with a raw HTML header (`<div align="center"><img><h1>…</h1></div>`); remark
 * parses such a block as a single mdast `html` node, and this element both
 * RENDERS it (sanitized) and preserves its exact source on save — see
 * `markdown-html-rules.ts`, which round-trips the raw string byte-for-byte
 * through an mdast `html` node (which remark stringifies without escaping).
 *
 * The block is VOID: its HTML is not rich-editable in place (it's a rendered
 * preview of source the editor owns as an opaque string). To change it, edit
 * the file in Source view — same contract as YAML frontmatter here.
 */
export interface HtmlElementNode {
  type: 'html'
  html: string
  children: [{ text: '' }]
}

export function HtmlElement(props: PlateElementProps) {
  const raw = (props.element as unknown as { html?: string }).html ?? ''
  // DOMPurify strips scripts/event handlers/etc. — the raw string is untrusted
  // file content. `sanitize` runs against the live DOM in the app and jsdom in
  // tests; both are fine. The sanitized output is display-only and never fed
  // back into serialization (that uses the untouched `html` string), so
  // sanitization can't corrupt the saved file.
  const clean = DOMPurify.sanitize(raw)

  const asset = useMarkdownAsset()
  const ref = useRef<HTMLDivElement>(null)

  // Local `<img src="./logo.png">` references (common in README headers) don't
  // resolve in the webview — the base URL is the dev server, not the file's
  // folder. After the sanitized HTML paints, rewrite each local image src to a
  // `data:` URL read from the workspace. Remote/data srcs are left untouched.
  // Runs whenever the HTML or the file's location changes; cancels late reads.
  useEffect(() => {
    const el = ref.current
    if (!el || !asset) return
    let cancelled = false
    for (const img of Array.from(el.querySelectorAll('img'))) {
      const src = img.getAttribute('src')
      if (!src) continue
      void loadLocalImage(asset, src).then((url) => {
        if (url && !cancelled) img.src = url
      })
    }
    return () => {
      cancelled = true
    }
  }, [clean, asset])

  // Anchor clicks are delegated with a NATIVE listener rather than a React
  // `onClick` prop, because the container is not itself a control — the
  // interactive elements are the `<a>`s inside HTML this component does not own
  // and cannot annotate. They are natively focusable and Enter on a focused
  // anchor dispatches a click, so keyboard users reach this path through the
  // anchor, not through the container; giving the container a `role` and key
  // handler of its own would advertise an interactive element that isn't one.
  // A plain anchor click inside the Tauri WKWebView navigates the whole app
  // view, so every one must be cancelled and routed to the OS browser.
  useEffect(() => {
    const el = ref.current
    if (!el) return
    const onClick = (e: MouseEvent) => {
      const anchor = (e.target as HTMLElement | null)?.closest('a')
      if (anchor) handleMarkdownAnchorClick(e, anchor.getAttribute('href'))
    }
    el.addEventListener('click', onClick)
    return () => el.removeEventListener('click', onClick)
  }, [])

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
