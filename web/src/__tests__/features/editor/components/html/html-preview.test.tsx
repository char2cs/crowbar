import { createElement } from 'react'
import { render, waitFor } from '@testing-library/react'
import { describe, expect, it, beforeAll, beforeEach, vi } from 'vitest'

const { readDocument } = vi.hoisted(() => ({ readDocument: vi.fn() }))
vi.mock('@/lib/api/documents', async (importOriginal) => ({
  ...(await importOriginal<typeof import('@/lib/api/documents')>()),
  readDocument,
}))

import { MONACO_LOAD_MS, openTestDocument } from '@/__tests__/__fixtures__/documents'
import { loadEditorHost } from '@/features/editor/host/editor-host'
import { workspaceRef, type DocumentRef } from '@/lib/api/documents'
import {
  windowPaneStore,
  resetWindowPaneStoreForTests,
} from '@/features/panes/stores/window-pane-store'
import { HtmlPreview } from '@/features/editor/components/html/html-preview'

async function previewOf(doc: DocumentRef, text: string) {
  await openTestDocument(doc, text, { name: 'a.html' })
  const { openContent } = windowPaneStore.getState().bufferActions
  openContent({ type: 'htmlPreview', source: doc, name: 'a.html (Preview)', content: '' })
  return render(createElement(HtmlPreview)).container
}

const frame = (container: HTMLElement) => container.querySelector('iframe')

beforeAll(() => loadEditorHost(), MONACO_LOAD_MS)

beforeEach(() => {
  // Task 26: panes/buffers are a window-level singleton now.
  resetWindowPaneStoreForTests()
  readDocument.mockReset()
  readDocument.mockImplementation(async (ref: DocumentRef) => ({ ref, text: 'png-bytes' }))
})

// Regression pin (Task 20): the preview frames potentially agent-generated /
// untrusted HTML. `allow-same-origin` COMBINED with `allow-scripts` lets that
// document reach the parent Crowbar origin and remove its own sandbox — so the
// sandbox string must never re-include it. This asserts on the real rendered
// iframe attribute, not the source text, so it also catches a runtime rebuild.
describe('HtmlPreview iframe sandbox', () => {
  it('never grants allow-same-origin', async () => {
    const container = await previewOf(workspaceRef('w1', 'a.html'), '<h1>hi</h1>')

    expect(frame(container)).not.toBeNull()
    expect(frame(container)?.getAttribute('sandbox') ?? '').not.toContain('allow-same-origin')
  })

  it('renders its own workspace’s source when another workspace has the same path', async () => {
    await openTestDocument(workspaceRef('w2', 'a.html'), '<h1>foreign</h1>')
    const container = await previewOf(workspaceRef('w1', 'a.html'), '<h1>own</h1>')

    await waitFor(() => expect(frame(container)?.getAttribute('srcdoc')).toContain('own'))
    expect(frame(container)?.getAttribute('srcdoc')).not.toContain('foreign')
  })
})

describe('HtmlPreview of a host document', () => {
  const host = (path: string): DocumentRef => ({ authority: 'host', path })

  it('runs no script and loads nothing but inline styles and data images', async () => {
    const container = await previewOf(host('/tmp/site/a.html'), '<!doctype html><h1>hi</h1>')

    await waitFor(() => expect(frame(container)?.getAttribute('srcdoc')).toContain('hi'))
    expect(frame(container)?.getAttribute('sandbox')).not.toContain('allow-scripts')
    expect(frame(container)?.getAttribute('srcdoc')).toMatch(
      /^<!doctype html><meta http-equiv="Content-Security-Policy" content="default-src 'none'; style-src 'unsafe-inline'; img-src data:">/,
    )
  })

  it('inlines assets from its own directory and nothing above it', async () => {
    const container = await previewOf(
      host('/tmp/site/a.html'),
      '<img src="img/ok.png"><img src="../secret.png"><img src="/etc/x.png">',
    )

    await waitFor(() =>
      expect(frame(container)?.getAttribute('srcdoc')).toContain('data:image/png'),
    )
    expect(readDocument.mock.calls.map(([ref]) => ref)).toEqual([host('/tmp/site/img/ok.png')])
  })
})
