import { createElement } from 'react'
import { render } from '@testing-library/react'
import { describe, expect, it } from 'vitest'
import { WorkspaceStoreContext } from '@/features/workspace/stores/workspace-context'
import { createWorkspaceStore } from '@/features/workspace/stores/workspace-store'
import { HtmlPreview } from '@/features/editor/components/html/html-preview'

// Regression pin (Task 20): the preview frames potentially agent-generated /
// untrusted HTML. `allow-same-origin` COMBINED with `allow-scripts` lets that
// document reach the parent Crowbar origin and remove its own sandbox — so the
// sandbox string must never re-include it. This asserts on the real rendered
// iframe attribute, not the source text, so it also catches a runtime rebuild.
describe('HtmlPreview iframe sandbox', () => {
  it('never grants allow-same-origin', () => {
    const store = createWorkspaceStore('w1')
    // Any active buffer makes `hasSourceBuffer` true so the iframe renders.
    store.getState().bufferActions.openContent({
      type: 'diff',
      path: '/repo/a.html',
      name: 'a.html',
      content: '<h1>hi</h1>',
    })

    const { container } = render(
      createElement(WorkspaceStoreContext.Provider, { value: store }, createElement(HtmlPreview)),
    )

    const iframe = container.querySelector('iframe')
    expect(iframe).not.toBeNull()
    expect(iframe?.getAttribute('sandbox') ?? '').not.toContain('allow-same-origin')
  })
})
