import { describe, expect, it, vi } from 'vitest'
import { buildHtmlPreviewDocument } from '@/features/editor/components/html/html-preview-document'
import { mimeForPath, toDataUrl } from '@/features/editor/lib/asset-data-url'

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

  // The desktop shell cancels every data: navigation, nested frames included,
  // so a frame's local document must arrive as srcdoc, not as a data: src.
  describe('nested frames', () => {
    const SVG = '<svg xmlns="http://www.w3.org/2000/svg"><rect width="4" height="4"/></svg>'
    const HOSTILE_SVG =
      '<svg xmlns="http://www.w3.org/2000/svg"><title>a "q" &amp; \'b\' &lt;c&gt; > d</title></svg>'
    const PNG = '\x89PNG\r\n'
    const files: Record<string, string> = { 'x.svg': SVG, 'hostile.svg': HOSTILE_SVG, 'y.png': PNG }
    const loadFile = async (reference: string) => {
      const mime = mimeForPath(reference)
      const bytes = files[reference]
      return mime && bytes !== undefined ? toDataUrl(mime, bytes) : null
    }
    const parse = (doc: string) => new DOMParser().parseFromString(doc, 'text/html')

    it('carries a local svg in srcdoc instead of a data: src', async () => {
      const doc = parse(
        await buildHtmlPreviewDocument('<iframe src="x.svg" title="chart"></iframe>', loadFile),
      )

      const frame = doc.querySelector('iframe')
      expect(frame?.hasAttribute('src')).toBe(false)
      expect(frame?.getAttribute('srcdoc')).toBe(SVG)
      expect(frame?.getAttribute('title')).toBe('chart')
    })

    it('round-trips quotes, ampersands and angle brackets without breaking later tags', async () => {
      const doc = parse(
        await buildHtmlPreviewDocument(
          '<iframe src="hostile.svg"></iframe><img alt="after" src="y.png">',
          loadFile,
        ),
      )

      expect(doc.querySelector('iframe')?.getAttribute('srcdoc')).toBe(HOSTILE_SVG)
      expect(doc.querySelector('img[alt="after"]')?.getAttribute('src')).toBe(
        toDataUrl('image/png', PNG),
      )
    })

    it('keeps the data: rewrite for a non-frame svg byte-for-byte', async () => {
      expect(await buildHtmlPreviewDocument('<img src="x.svg">', loadFile)).toBe(
        `<img src="${toDataUrl('image/svg+xml', SVG)}">`,
      )
    })

    it('keeps the data: src when the loaded text cannot be decoded', async () => {
      const malformed = 'data:image/svg+xml;charset=utf-8,%E0%A4%A'
      const doc = parse(
        await buildHtmlPreviewDocument('<iframe src="bad.svg"></iframe>', async () => malformed),
      )

      expect(doc.querySelector('iframe')?.getAttribute('src')).toBe(malformed)
      expect(doc.querySelector('iframe')?.hasAttribute('srcdoc')).toBe(false)
    })

    it('leaves remote and unloadable frame references untouched', async () => {
      const html = '<iframe src="https://x.dev/a.svg"></iframe><iframe src="missing.svg"></iframe>'
      expect(await buildHtmlPreviewDocument(html, loadFile)).toBe(html)
    })

    it('keeps a frame’s own srcdoc', async () => {
      const doc = parse(
        await buildHtmlPreviewDocument(
          '<iframe src="x.svg" srcdoc="&lt;p&gt;own&lt;/p&gt;"></iframe>',
          loadFile,
        ),
      )

      expect(doc.querySelector('iframe')?.getAttribute('srcdoc')).toBe('<p>own</p>')
    })
  })
})
