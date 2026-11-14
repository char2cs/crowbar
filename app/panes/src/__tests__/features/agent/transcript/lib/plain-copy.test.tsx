import { fireEvent, render } from '@testing-library/react'
import { describe, expect, it } from 'vitest'
import { copyWithoutAnchors } from '@/features/agent/transcript/lib/plain-copy'

/** Copies what `select` selects inside a block that holds `text`. */
function copy(text: string) {
  const { container } = render(
    <div onCopy={copyWithoutAnchors}>
      <pre>{text}</pre>
    </div>,
  )
  const block = container.querySelector('pre')!
  getSelection()!.selectAllChildren(block)
  const data = new Map<string, string>()
  const clipboardData = { setData: (type: string, value: string) => data.set(type, value) }
  fireEvent.copy(block, { clipboardData })
  return data
}

describe('copyWithoutAnchors', () => {
  it('copies an empty line’s caret anchor as nothing', () => {
    const data = copy('first\n﻿\nthird')
    expect(data.get('text/plain')).toBe('first\n\nthird')
    expect(data.get('text/html')).toBe('first\n\nthird')
  })

  it('leaves a copy with no anchor to the browser', () => {
    expect(copy('plain').size).toBe(0)
  })
})
