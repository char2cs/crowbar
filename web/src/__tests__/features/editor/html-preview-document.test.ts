import { describe, expect, it, vi } from 'vitest'
import type { DocumentRef } from '@/lib/api/documents'
import {
  buildHostPreviewDocument,
  buildHtmlPreviewDocument,
} from '@/features/editor/components/html/html-preview-document'

// The preview frame is sandboxed with an opaque origin and loaded from
// srcdoc: a relative path resolves against nothing, so local assets must be
// inlined. (The old path-to-URL conversion was the identity function, so
// every relative asset in a previewed page was broken.)
const loadAsset = vi.fn(async (reference: string) => `data:x;ref=${reference}`)

describe('buildHtmlPreviewDocument', () => {
  it('inlines relative and root-relative resource references', async () => {
    const html =
      '<img src="logo.png"><script src="/src/main.js"></script><video poster=\'./p.jpg\'></video>'

    const doc = await buildHtmlPreviewDocument(html, loadAsset)

    expect(doc).toContain('src="data:x;ref=logo.png"')
    expect(doc).toContain('src="data:x;ref=/src/main.js"')
    expect(doc).toContain('poster="data:x;ref=./p.jpg"')
  })

  it('inlines stylesheet links but leaves navigation links alone', async () => {
    const doc = await buildHtmlPreviewDocument(
      '<link rel="stylesheet" href="style.css"><a href="other.html">x</a>',
      loadAsset,
    )

    expect(doc).toContain('href="data:x;ref=style.css"')
    expect(doc).toContain('<a href="other.html">')
  })

  it('rewrites each srcset candidate and keeps descriptors', async () => {
    const doc = await buildHtmlPreviewDocument('<img srcset="a.png 1x, b.png 2x">', loadAsset)
    expect(doc).toContain('srcset="data:x;ref=a.png 1x, data:x;ref=b.png 2x"')
  })

  it('never touches remote, data, protocol-relative or anchor references', async () => {
    loadAsset.mockClear()
    const html =
      '<img src="https://x.dev/a.png"><img src="data:image/png;base64,AA"><img src="//cdn/a.png"><link href="#top">'

    expect(await buildHtmlPreviewDocument(html, loadAsset)).toBe(html)
    expect(loadAsset).not.toHaveBeenCalled()
  })

  it('leaves a reference untouched when it cannot be loaded', async () => {
    const failing = vi.fn(async () => {
      throw new Error('not found')
    })
    const html = '<img src="missing.png">'
    expect(await buildHtmlPreviewDocument(html, failing)).toBe(html)
  })

  it('loads each distinct reference once', async () => {
    loadAsset.mockClear()
    await buildHtmlPreviewDocument('<img src="a.png"><img src="a.png">', loadAsset)
    expect(loadAsset).toHaveBeenCalledTimes(1)
  })
})

describe('buildHostPreviewDocument', () => {
  const files: Record<string, string> = {
    '/site/css/a.css': 'h1{color:red}',
    '/x.css': 'h1{color:blue}',
    '/site/app.js': 'alert(1)',
  }
  const read = vi.fn(async (ref: DocumentRef) => files[ref.path] ?? '')
  const build = (html: string) =>
    buildHostPreviewDocument(html, { authority: 'host', path: '/site/index.html' }, read)

  it('inlines its own stylesheets as <style> and reads nothing its policy blocks', async () => {
    read.mockClear()
    const doc = await build(
      '<link rel="stylesheet" href="css/a.css"><link rel="stylesheet" href="../x.css"><script src="app.js"></script>',
    )

    expect(doc).toContain('<style>h1{color:red}</style>')
    expect(doc).toContain('<link rel="stylesheet" href="../x.css">')
    expect(doc).toContain('<script src="app.js"></script>')
    expect(read.mock.calls.map(([ref]) => ref.path)).toEqual(['/site/css/a.css'])
  })

  it('keeps a stylesheet’s media query', async () => {
    const doc = await build('<link media="print" rel="stylesheet" href="css/a.css">')
    expect(doc).toContain('<style media="print">h1{color:red}</style>')
  })
})
