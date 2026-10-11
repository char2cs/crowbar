import { readdirSync, readFileSync } from 'node:fs'
import { dirname, join } from 'node:path'
import { fileURLToPath } from 'node:url'
import { cleanup, render } from '@testing-library/react'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { ProviderIcon } from '@/components/ui/provider-icon'

afterEach(cleanup)

// The icon is a raw descriptor string a remote daemon serves verbatim (any
// paired client can upload one), so it is attacker-controlled markup.
function renderIcon(svg: string): HTMLElement {
  const { container } = render(<ProviderIcon svg={svg} />)
  const host = container.querySelector<HTMLElement>('[data-provider-icon]')
  if (!host) throw new Error('ProviderIcon rendered no host span')
  return host
}

const ACTIVE_TAGS = [
  'script',
  'foreignobject',
  'iframe',
  'img',
  'object',
  'embed',
  'style',
  'use',
  'animate',
  'set',
  'animatemotion',
  'animatetransform',
  'image',
  'a',
  'switch',
  'math',
]

function expectInert(host: HTMLElement): void {
  for (const element of host.querySelectorAll('*')) {
    const tag = element.localName.toLowerCase()
    expect(ACTIVE_TAGS, `<${tag}> survived`).not.toContain(tag)
    for (const attribute of element.attributes) {
      const name = attribute.localName.toLowerCase()
      const value = attribute.value.replace(/\s/g, '').toLowerCase()
      expect(name.startsWith('on'), `${tag}[${name}] event handler survived`).toBe(false)
      expect(name, `${tag}[style] survived`).not.toBe('style')
      expect(value, `${tag}[${name}] script/data URL survived`).not.toMatch(/javascript:|data:/)
      if (name === 'href') expect(value.startsWith('#'), `${tag}[${name}]=${value}`).toBe(true)
      expect(value, `${tag}[${name}] external url() survived`).not.toMatch(/url\((?!['"]?#)/)
    }
  }
}

const HOSTILE: Array<[string, string]> = [
  ['trailing <img onerror>', '<svg viewBox="0 0 1 1"></svg><img src=x onerror="window.__pwned=1">'],
  ['<script>', '<svg viewBox="0 0 1 1"><script>window.__pwned=1</script></svg>'],
  [
    '<foreignObject><iframe>',
    '<svg><foreignObject><iframe src="https://evil.example/"></iframe></foreignObject></svg>',
  ],
  ['onload on the root', '<svg onload="window.__pwned=1"><path d="M0 0"/></svg>'],
  ['onclick on a shape', '<svg><path d="M0 0" onclick="window.__pwned=1"/></svg>'],
  ['javascript: href', '<svg><a href="javascript:window.__pwned=1"><path d="M0 0"/></a></svg>'],
  [
    'data: xlink:href',
    '<svg xmlns:xlink="http://www.w3.org/1999/xlink"><linearGradient id="g" xlink:href="data:image/svg+xml,&lt;svg onload=alert(1)&gt;"/></svg>',
  ],
  ['external <use href>', '<svg><use href="https://evil.example/x.svg#a"/></svg>'],
  [
    'external gradient href',
    '<svg><linearGradient id="g" href="https://evil.example/x.svg#a"/></svg>',
  ],
  ['external paint url()', '<svg><path d="M0 0" fill="url(https://evil.example/x.svg#p)"/></svg>'],
  [
    '<animate> rewriting href',
    '<svg><a><animate attributeName="href" values="javascript:alert(1)"/><text>x</text></a></svg>',
  ],
  ['<set> rewriting href', '<svg><set attributeName="href" to="javascript:alert(1)"/></svg>'],
  ['<style> @import', '<svg><style>@import url(https://evil.example/x.css);</style></svg>'],
  ['style attribute', '<svg><path d="M0 0" style="background:url(https://evil.example/)"/></svg>'],
  [
    'mXSS: <p><style> inside svg',
    '<svg><p><style><a title="</style><img src onerror=alert(1)>"></a></style></p></svg>',
  ],
  [
    'mXSS: CDATA in <desc>',
    '<svg><desc><![CDATA[</desc><script>window.__pwned=1</script>]]></desc></svg>',
  ],
  [
    'mXSS: math/mglyph namespace confusion',
    '<svg><math><mtext><table><mglyph><style><!--</style><img title="--&gt;&lt;img src=1 onerror=alert(1)&gt;">',
  ],
  [
    '<image> external load',
    '<svg><image href="https://evil.example/track.png" width="1" height="1"/></svg>',
  ],
]

describe('ProviderIcon', () => {
  it.each(HOSTILE)('renders nothing active for %s', (_label, svg) => {
    expectInert(renderIcon(svg))
  })

  it('keeps a same-document gradient reference and the paint that uses it', () => {
    const host = renderIcon(
      '<svg viewBox="0 0 24 24"><defs><linearGradient id="g"><stop offset="0" stop-color="#000"/></linearGradient><linearGradient id="h" href="#g"/></defs><path d="M0 0h24v24H0z" fill="url(#h)"/></svg>',
    )
    expect(host.querySelector('#h')?.getAttribute('href')).toBe('#g')
    expect(host.querySelector('path')?.getAttribute('fill')).toBe('url(#h)')
  })

  it('renders an empty host, never the raw string, when nothing safe is left', () => {
    for (const svg of ['', 'not markup at all <', '<img src=x onerror="window.__pwned=1">']) {
      const host = renderIcon(svg)
      expect(host.innerHTML, JSON.stringify(svg)).toBe('')
    }
  })

  it('cannot restyle the app around it through class or style hooks', () => {
    const host = renderIcon('<svg class="fixed inset-0 z-50"><path d="M0 0" class="x"/></svg>')
    expect(host.querySelector('[class]')).toBeNull()
  })

  // A real overflow needs a ~5000-deep document: seconds of jsdom parsing, and the depth that
  // overflows shifts with JIT warmth. So the serialiser's RangeError is injected on a tiny icon.
  it('renders an empty host instead of throwing when the icon nests too deep to serialise', () => {
    const serialise = vi.spyOn(Element.prototype, 'outerHTML', 'get').mockImplementation(() => {
      throw new RangeError('Maximum call stack size exceeded')
    })
    try {
      const svg = '<svg viewBox="0 0 3 3"><path d="M2 2"/></svg>'
      expect(renderIcon(svg).innerHTML).toBe('')
      expect(renderIcon(svg).innerHTML).toBe('')
      expect(
        serialise,
        'a failed icon is cached, not re-sanitised per render',
      ).toHaveBeenCalledTimes(1)
    } finally {
      serialise.mockRestore()
    }
  })

  it('renders an empty host instead of throwing when sanitising fails', () => {
    const parse = vi.spyOn(DOMParser.prototype, 'parseFromString').mockImplementation(() => {
      throw new Error('parser exploded')
    })
    try {
      expect(renderIcon('<svg viewBox="0 0 2 2"><path d="M1 1"/></svg>').innerHTML).toBe('')
    } finally {
      parse.mockRestore()
    }
  })
})

// The descriptors the daemon ships, read from the repo so a changed icon is
// checked as it is, not as a stale copy.
const DESCRIPTORS = join(
  dirname(fileURLToPath(import.meta.url)),
  '../../../../../api/internal/engine/agents/internal/protocol/internal/descriptor/descriptors-v3',
)

function shippedIcons(): Array<[string, string]> {
  return readdirSync(DESCRIPTORS)
    .filter((name) => name.endsWith('.yaml'))
    .flatMap((name): Array<[string, string]> => {
      const match = /^icon: '(.*)'$/m.exec(readFileSync(join(DESCRIPTORS, name), 'utf8'))
      return match?.[1] ? [[name, match[1].replace(/''/g, "'")]] : []
    })
}

const SHAPES = 'path, circle, ellipse, line, polyline, polygon, rect'

describe('ProviderIcon with the shipped descriptor icons', () => {
  const icons = shippedIcons()

  it('finds the shipped icons (so the cases below are not vacuous)', () => {
    expect(icons.map(([name]) => name)).toEqual(
      expect.arrayContaining(['claude.yaml', 'codex.yaml']),
    )
  })

  it.each(icons)('%s keeps its svg root, viewBox and every shape', (_name, svg) => {
    const original = new DOMParser().parseFromString(svg, 'image/svg+xml').documentElement
    const host = renderIcon(svg)
    const root = host.firstElementChild
    expect(root?.localName).toBe('svg')
    expect(root?.getAttribute('viewBox')).toBe(original.getAttribute('viewBox'))
    expect(root?.getAttribute('fill')).toBe(original.getAttribute('fill'))
    expect(host.querySelectorAll(SHAPES)).toHaveLength(original.querySelectorAll(SHAPES).length)
    expect(host.querySelectorAll(SHAPES).length).toBeGreaterThan(0)
    expect(host.querySelector('path')?.getAttribute('d')).toBe(
      original.querySelector('path')?.getAttribute('d'),
    )
  })
})
