'use client'

import { useEffect, useState } from 'react'
import { createPlatePlugin, PlateElement, type PlateElementProps } from 'platejs/react'
import { loadLocalImage, useMarkdownAsset } from './markdown-asset'

export interface MarkdownImageNode {
  type: 'img'
  url?: string
  caption?: Array<{ text?: string }> | string
}

export function captionToAlt(caption: MarkdownImageNode['caption']): string {
  if (Array.isArray(caption)) return caption.map((c) => c?.text ?? '').join('')
  return typeof caption === 'string' ? caption : ''
}

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
      if (!controller.signal.aborted) setResolvedSrc(data ?? url)
    })
    return () => controller.abort()
  }, [asset, url])

  return resolvedSrc
}

function MarkdownImageElement(props: PlateElementProps) {
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

export const MarkdownImageKit = [
  createPlatePlugin({
    key: 'img',
    node: {
      isElement: true,
      isVoid: true,
      component: MarkdownImageElement,
    },
  }),
]
