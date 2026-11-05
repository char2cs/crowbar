'use client'

import { useEffect, useState } from 'react'
import { createPlatePlugin, PlateElement, type PlateElementProps } from 'platejs/react'
import { loadLocalImage, useMarkdownAsset } from './markdown-asset'

/** Renderer for the `img` node @platejs/markdown already converts to and from `![alt](src)`
 *  (alt as `caption`); without it this editor dropped images. Local srcs load as data URLs
 *  against the file's folder, like the raw-HTML `<img>` path. */
export interface MarkdownImageNode {
  type: 'img'
  url?: string
  /** alt text, stored by @platejs as an array of text nodes (or a string). */
  caption?: Array<{ text?: string }> | string
}

export function captionToAlt(caption: MarkdownImageNode['caption']): string {
  if (Array.isArray(caption)) return caption.map((c) => c?.text ?? '').join('')
  return typeof caption === 'string' ? caption : ''
}

/** `url` resolved against the current `MarkdownAssetContext` (a local image becomes a data URL;
 *  otherwise `url` unchanged). Shared with ChatAttachmentImageElement, which differs in chrome. */
export function useResolvedImageSrc(url: string): string {
  const asset = useMarkdownAsset()
  const [resolvedSrc, setResolvedSrc] = useState(url)

  useEffect(() => {
    if (!asset) {
      setResolvedSrc(url)
      return
    }
    const controller = new AbortController()
    void loadLocalImage(asset, url, controller.signal).then((data) => {
      // `data` is a data: URL for a resolvable local image, or null for a
      // remote/`data:`/unresolvable src — in which case the original loads fine.
      if (!controller.signal.aborted) setResolvedSrc(data ?? url)
    })
    return () => controller.abort()
  }, [asset, url])

  return resolvedSrc
}

export function MarkdownImageElement(props: PlateElementProps) {
  const element = props.element as unknown as MarkdownImageNode
  const url = element.url ?? ''
  const alt = captionToAlt(element.caption)
  const resolvedSrc = useResolvedImageSrc(url)

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

/** Void `img` element plugin: the renderer only. No markdown rules, since the built-in
 *  conversion is what keeps `![alt](url)` round-tripping. */
export const MarkdownImageKit = [
  createPlatePlugin({
    key: 'img',
    node: {
      isElement: true,
      // Block, as @platejs deserializes an `image`: one alone on a line round-trips exactly, one
      // among text is promoted to its own line, never dropped.
      isVoid: true,
      component: MarkdownImageElement,
    },
  }),
]
