import type { ClipboardEvent } from 'react'

/** The editor's caret anchor: an empty rendered line holds one, invisible but copied. */
const ANCHOR = /\uFEFF/g

/** A copy out of the transcript carries the text a reader sees, without the anchors
 *  its empty lines hold; any other copy is left to the browser. */
export function copyWithoutAnchors(event: ClipboardEvent<HTMLElement>) {
  const selection = window.getSelection()
  const text = selection?.toString() ?? ''
  if (!selection || selection.rangeCount === 0 || !text.includes('\uFEFF')) return
  const html = document.createElement('div')
  html.append(selection.getRangeAt(0).cloneContents())
  event.clipboardData.setData('text/plain', text.replace(ANCHOR, ''))
  event.clipboardData.setData('text/html', html.innerHTML.replace(ANCHOR, ''))
  event.preventDefault()
}
