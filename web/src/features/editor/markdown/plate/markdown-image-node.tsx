'use client'

import { useEffect, useState } from 'react'
import { createPlatePlugin, PlateElement, type PlateElementProps } from 'platejs/react'
import { loadLocalImage, useMarkdownAsset } from './markdown-asset'

/**
 * Renders markdown `![alt](src "title")` images. @platejs/markdown already
 * deserializes an mdast `image` into a Plate `img` node (`{ type:'img', url,
 * caption }`, alt stored as `caption`) and serializes it back — but this editor
 * ships no image kit, so that node had no component and was dropped. This adds
 * the missing renderer only; the markdown conversion stays @platejs's, so
 * `![alt](url)` round-trips byte-for-byte.
 *
 * Inline VOID element (an mdast `image` is phrasing — it can sit inside a
 * paragraph next to text), rendered as a real `<img>`. Local `src`s (README
 * screenshots/logos) resolve against the file's folder and load as data URLs,
 * exactly like the raw-HTML `<img>` path; remote/`data:` srcs load themselves.
 */
interface MarkdownImageNode {
  type: 'img'
  url?: string
  /** alt text, stored by @platejs as an array of text nodes (or a string). */
  caption?: Array<{ text?: string }> | string
}

function captionToAlt(caption: MarkdownImageNode['caption']): string {
  if (Array.isArray(caption)) return caption.map((c) => c?.text ?? '').join('')
  return typeof caption === 'string' ? caption : ''
}

export function MarkdownImageElement(props: PlateElementProps) {
  const element = props.element as unknown as MarkdownImageNode
  const url = element.url ?? ''
  const alt = captionToAlt(element.caption)
  const asset = useMarkdownAsset()
  const [resolvedSrc, setResolvedSrc] = useState(url)

  useEffect(() => {
    let cancelled = false
    if (!asset) {
      setResolvedSrc(url)
      return
    }
    void loadLocalImage(asset, url).then((data) => {
      // `data` is a data: URL for a resolvable local image, or null for a
      // remote/`data:`/unresolvable src — in which case the original loads fine.
      if (!cancelled) setResolvedSrc(data ?? url)
    })
    return () => {
      cancelled = true
    }
  }, [asset, url])

  return (
    <PlateElement {...props} className="inline-block">
      <span contentEditable={false}>
        <img
          src={resolvedSrc}
          alt={alt}
          className="markdown-image inline-block h-auto max-w-full rounded"
        />
      </span>
      {props.children}
    </PlateElement>
  )
}

/**
 * Inline void `img` element plugin — supplies the renderer for the `img` node
 * @platejs/markdown already emits. No markdown rules: the built-in
 * image↔markdown conversion is what keeps `![alt](url)` round-tripping.
 */
export const MarkdownImageKit = [
  createPlatePlugin({
    key: 'img',
    node: {
      isElement: true,
      // Block, matching how @platejs deserializes an `image` (it lifts one out
      // of a paragraph into its own block). An image alone on a line — the
      // common README case — round-trips exactly; an image inline among text is
      // promoted to its own line, a harmless reflow, never dropped.
      isVoid: true,
      component: MarkdownImageElement,
    },
  }),
]
