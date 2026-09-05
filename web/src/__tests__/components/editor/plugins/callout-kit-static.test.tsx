import { render } from '@testing-library/react'
import type { Value } from 'platejs'
import type { AnyPlatePlugin } from 'platejs/react'
import { Plate, PlateContent, ParagraphPlugin, usePlateEditor } from 'platejs/react'
import { createStaticEditor, PlateStatic } from 'platejs/static'
import { describe, expect, it } from 'vitest'

import { CalloutKit } from '@/components/editor/plugins/callout-kit'
import { CalloutKitStatic } from '@/components/editor/plugins/callout-kit-static'
import { LinkKit, LinkKitStatic } from '@/components/editor/plugins/link-kit'
import { ParagraphElement } from '@/components/ui/paragraph-node'

/**
 * Parity between the interactive editor (mounted the same way
 * MarkdownMessage mounts it — see markdown-message.test.tsx) and the
 * PlateStatic read path this plan is introducing. Both plugin sets share
 * the same node component, so any divergence here is a real bug, not a
 * fixture mismatch to special-case away.
 */

function InteractiveHarness({ plugins, value }: { plugins: AnyPlatePlugin[]; value: Value }) {
  // Same construction MarkdownMessage uses: usePlateEditor(..., []) then a
  // read-only Plate + PlateContent, not a bespoke test-only setup.
  const editor = usePlateEditor({ plugins, value }, [])
  return (
    <Plate editor={editor} readOnly>
      <PlateContent readOnly tabIndex={-1} />
    </Plate>
  )
}

describe('CalloutKitStatic', () => {
  it('renders the same visible content as the interactive callout', () => {
    const value: Value = [
      { type: 'callout', icon: '⚠️', children: [{ text: 'Careful with this one.' }] },
    ]

    const interactive = render(<InteractiveHarness plugins={CalloutKit} value={value} />)
    const staticEditor = createStaticEditor({ plugins: CalloutKitStatic, value })
    const staticRender = render(<PlateStatic editor={staticEditor} />)

    expect(staticRender.container.textContent).toBe(interactive.container.textContent)
    expect(staticRender.container.querySelector('[data-slate-node]')?.textContent).toContain(
      'Careful with this one.',
    )
  })
})

describe('LinkKitStatic', () => {
  it('renders the same visible content, including href, as the interactive link', () => {
    const plugins = [ParagraphPlugin.withComponent(ParagraphElement), ...LinkKit]
    const staticPlugins = [ParagraphPlugin.withComponent(ParagraphElement), ...LinkKitStatic]
    const value: Value = [
      {
        type: 'p',
        children: [
          { text: 'Go to ' },
          { type: 'a', url: 'https://example.com', children: [{ text: 'the site' }] },
          { text: '.' },
        ],
      },
    ]

    const interactive = render(<InteractiveHarness plugins={plugins} value={value} />)
    const staticEditor = createStaticEditor({ plugins: staticPlugins, value })
    const staticRender = render(<PlateStatic editor={staticEditor} />)

    expect(staticRender.container.textContent).toBe(interactive.container.textContent)

    const staticAnchor = staticRender.container.querySelector('a')
    const interactiveAnchor = interactive.container.querySelector('a')
    // sanitizeUrl normalizes the bare origin with a trailing slash — assert
    // parity against the interactive render rather than a hand-typed literal.
    expect(staticAnchor?.getAttribute('href')).toContain('example.com')
    expect(staticAnchor?.getAttribute('href')).toBe(interactiveAnchor?.getAttribute('href'))

    // The one thing LinkKitStatic must NOT render: the interactive-only
    // floating toolbar (LinkFloatingToolbar calls useEditorRef(), which is
    // invalid outside an interactive editor — dropping it is the whole fix).
    expect(staticRender.container.querySelector('[data-radix-popper-content-wrapper]')).toBeNull()
  })
})
