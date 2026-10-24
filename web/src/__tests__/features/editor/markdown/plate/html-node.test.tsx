import { useMemo } from 'react'
import { beforeEach, describe, expect, it } from 'vitest'
import { render, screen } from '@testing-library/react'
import { Plate, PlateContent, usePlateEditor } from 'platejs/react'
import { markdownPlugins } from '@/features/editor/markdown/plate/markdown-plugins'
import { markdownToPlateValue } from '@/features/editor/markdown/plate/markdown-serialization'

// One `<div>` with no blank line parses as a single mdast `html` node, so every
// payload below reaches HtmlElement through the same path a README header takes.
const HOSTILE_HTML = `<div>
<img src=x onerror="window.__xss=1">
<script>window.__xss=1</script>
<a href="javascript:window.__xss=1">x</a>
<b>kept</b>
</div>
`

const xssFlag = () => (window as unknown as { __xss?: number }).__xss

function MarkdownDocument({ markdown }: { markdown: string }) {
  const value = useMemo(() => markdownToPlateValue(markdown), [markdown])
  const editor = usePlateEditor({ plugins: markdownPlugins, value })
  return (
    <Plate editor={editor}>
      <PlateContent />
    </Plate>
  )
}

beforeEach(() => {
  delete (window as unknown as { __xss?: number }).__xss
})

async function renderBlock() {
  const { container } = render(<MarkdownDocument markdown={HOSTILE_HTML} />)
  await screen.findByText('kept')
  const block = container.querySelector('.markdown-html-block')
  if (!block) throw new Error('raw HTML block did not render')
  return block
}

describe('HtmlElement (raw HTML block in a markdown file)', () => {
  it('renders no script element', async () => {
    const block = await renderBlock()

    expect(block.querySelector('script')).toBeNull()
    expect(xssFlag()).toBeUndefined()
  })

  it('renders no event-handler attribute', async () => {
    const block = await renderBlock()

    const handlers = Array.from(block.querySelectorAll('*')).flatMap((el) =>
      el.getAttributeNames().filter((name) => name.startsWith('on')),
    )
    expect(handlers).toEqual([])
    expect(block.querySelector('[onerror]')).toBeNull()
    expect(xssFlag()).toBeUndefined()
  })

  it('renders no javascript: link', async () => {
    const block = await renderBlock()

    expect(block.querySelector('a[href^="javascript:" i]')).toBeNull()
    expect(xssFlag()).toBeUndefined()
  })

  it('keeps the harmless markup', async () => {
    const block = await renderBlock()

    const kept = block.querySelector('b')
    expect(kept?.textContent).toBe('kept')
  })
})
