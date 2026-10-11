import { cleanup, render } from '@testing-library/react'
import { afterEach, describe, expect, it } from 'vitest'
import { MarkdownMessageStatic } from '@/features/agent/transcript/plate/markdown-message-static'

afterEach(cleanup)

function htmlBlock(html: string): Element {
  const { container } = render(<MarkdownMessageStatic>{html}</MarkdownMessageStatic>)
  const block = container.querySelector('.markdown-html-block')
  if (!block) throw new Error('no raw HTML block rendered')
  return block
}

describe('a raw HTML block', () => {
  it('keeps a README header as written', () => {
    const block = htmlBlock(
      [
        '<div align="center">',
        '<img src="https://example.com/logo.png" alt="Logo" width="120">',
        '<h1>Title</h1>',
        '<a href="https://tauri.app/">Tauri</a>',
        '<details><summary>More</summary>Body</details>',
        '</div>',
      ].join('\n'),
    )
    expect(block.querySelector('div[align="center"] img[width="120"]')).not.toBeNull()
    expect(block.querySelector('h1')?.textContent).toBe('Title')
    expect(block.querySelector('a[href="https://tauri.app/"]')).not.toBeNull()
    expect(block.querySelector('details summary')).not.toBeNull()
  })

  // It comes from an agent's message or a file on another host: nothing in it may restyle the
  // app around it, take what the user types, or navigate other than through a link click.
  it.each([
    ['a <style> sheet', '<div>x</div><style>*{display:none}</style>', 'style'],
    ['a style attribute', '<div style="position:fixed;inset:0;z-index:9999">x</div>', '[style]'],
    ['an app class', '<div class="fixed inset-0 z-50">x</div>', '[class]'],
    [
      'a form',
      '<form action="https://evil.example/"><input name="code"><button>Pair</button></form>',
      'form, input, button',
    ],
    [
      'a textarea or select',
      '<div><textarea></textarea><select><option>a</option></select></div>',
      'textarea, select',
    ],
    [
      'an image-map area',
      '<div><img usemap="#m"><map name="m"><area href="https://evil.example/" shape="rect"></map></div>',
      'map, area',
    ],
    [
      'inline SVG',
      '<div><svg><a href="https://evil.example/"><text>x</text></a></svg></div>',
      'svg, a',
    ],
  ])('drops %s', (_label, html, selector) => {
    expect(htmlBlock(html).querySelector(selector)).toBeNull()
  })
})
