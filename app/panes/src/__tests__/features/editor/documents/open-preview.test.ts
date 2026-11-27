import { afterEach, describe, expect, it, vi } from 'vitest'

const { openDocument } = vi.hoisted(() => ({ openDocument: vi.fn(async () => {}) }))
vi.mock('@/features/editor/documents/open-document', () => ({ openDocument }))

import { installPreviewOpener, openPreview } from '@/features/editor/documents/open-preview'
import { workspaceRef } from '@/lib/api/documents'

const DOC = workspaceRef('ws1', 'src/a.ts')

afterEach(() => {
  vi.clearAllMocks()
})

describe('openPreview', () => {
  it('in the app opens the document as this page’s own preview tab', async () => {
    await openPreview(DOC)
    expect(openDocument).toHaveBeenCalledExactlyOnceWith(DOC, { preview: true })
  })

  it('hands the document to the installed opener instead', async () => {
    const opener = vi.fn(async () => {})
    installPreviewOpener(opener)
    await openPreview(DOC)
    expect(opener).toHaveBeenCalledExactlyOnceWith(DOC)
    expect(openDocument).not.toHaveBeenCalled()
  })
})
