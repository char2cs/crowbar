'use client'

import { useEffect, useRef } from 'react'
import DOMPurify from 'dompurify'
import { createPlatePlugin, PlateElement, type PlateElementProps } from 'platejs/react'
import { handleMarkdownAnchorClick, useLinkWorkspace } from '@/lib/markdown-link'
import { loadLocalImage, useMarkdownAsset } from './markdown-asset'

const HTML_BLOCK_POLICY = {
  USE_PROFILES: { html: true },
  FORBID_TAGS: ['style', 'form', 'input', 'button', 'textarea', 'select', 'option', 'map', 'area'],
  FORBID_ATTR: ['style', 'class', 'action', 'formaction'],
}

function HtmlElement(props: PlateElementProps) {
  const raw = (props.element as unknown as { html?: string }).html ?? ''
  const clean = DOMPurify.sanitize(raw, HTML_BLOCK_POLICY)

  const asset = useMarkdownAsset()
  const linkWorkspace = useLinkWorkspace()
  const ref = useRef<HTMLDivElement>(null)

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
