import { describe, expect, it } from 'vitest'
import { MONACO_LOAD_MS } from '@/__tests__/__fixtures__/documents'
import { documentUri } from '@/features/editor/documents/document-ref'
import { installAppEditor } from '@/features/editor/host/app-editor'
import { loadEditorHost } from '@/features/editor/host/editor-host'
import { workspaceRef } from '@/lib/api/documents'

describe('installAppEditor', () => {
  it(
    'gives the app a Monaco host that opens a document in its path’s language',
    async () => {
      installAppEditor()
      const host = await loadEditorHost()
      expect(host.languageOf(documentUri(workspaceRef('ws1', 'src/a.ts')))).toBe('typescript')
    },
    MONACO_LOAD_MS,
  )
})
