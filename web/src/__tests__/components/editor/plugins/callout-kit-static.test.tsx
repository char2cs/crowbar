import { render } from '@testing-library/react'
import type { Value } from 'platejs'
import type { AnyPlatePlugin } from 'platejs/react'
import { Plate, PlateContent, usePlateEditor } from 'platejs/react'
import { createStaticEditor, PlateStatic } from 'platejs/static'
import { describe, expect, it } from 'vitest'

import { CalloutKit } from '@/components/editor/plugins/callout-kit'
import { CalloutKitStatic } from '@/components/editor/plugins/callout-kit-static'

/** Parity between the interactive editor (mounted as MarkdownMessage mounts it) and the PlateStatic
 *  read path. Both share one node component, so a divergence is a real bug, not a fixture issue. */

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
