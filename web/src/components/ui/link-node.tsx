'use client'

import type { TLinkElement } from 'platejs'
import type { PlateElementProps } from 'platejs/react'

import { getLinkAttributes } from '@platejs/link'
import { PlateElement } from 'platejs/react'

import { cn } from '@/lib/utils'
import { handleMarkdownAnchorClick } from '@/lib/markdown-link'
import { inlineSuggestionVariants } from '@/lib/suggestion'

export function LinkElement(props: PlateElementProps<TLinkElement>) {
  const linkAttributes = getLinkAttributes(props.editor, props.element)
  return (
    <PlateElement
      {...props}
      as="a"
      className={cn(
        'font-medium text-primary underline decoration-primary underline-offset-4',
        inlineSuggestionVariants(),
      )}
      attributes={{
        ...props.attributes,
        ...linkAttributes,
        // A bare <a> click navigates the whole WKWebView; route external URLs
        // to the OS browser and cancel the in-app navigation instead.
        onClick: (e) => {
          handleMarkdownAnchorClick(e, linkAttributes.href)
        },
        onMouseOver: (e) => {
          e.stopPropagation()
        },
      }}
    >
      {props.children}
    </PlateElement>
  )
}
