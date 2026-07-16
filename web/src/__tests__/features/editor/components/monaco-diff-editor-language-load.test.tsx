import { createElement } from 'react'
import { render } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'
import { WorkspaceStoreContext } from '@/features/workspace/stores/workspace-context'
import { createWorkspaceStore } from '@/features/workspace/stores/workspace-store'

// Diff-surface counterpart of the loader tests in
// __tests__/features/editor/monaco/language-contributions.test.ts: the git
// diff viewer / branch review render DiffMonacoEditor directly (no code pane,
// so no langForUri call ever fires), which means the DIFF surface itself must
// kick off the on-demand grammar load or diffs render plaintext for any
// language the user hasn't already opened in a code pane. This pins that
// wiring: the component calls loadLanguageForPath(<buffer path>) on mount.
//
// `language-contributions` is mocked (spy) so the assertion is exactly "the
// component initiated the load for this path" — the loader's own behavior
// (dedupe, registration, unknown extensions) is covered by the module-level
// suite above with real monaco. Everything else here is real: real workspace
// store, real monaco standalone editor construction under jsdom (proven by
// open-file-real-monaco-flow.test.ts).
const { loadLanguageForPathFn } = vi.hoisted(() => ({
  loadLanguageForPathFn: vi.fn((_path: string) => Promise.resolve()),
}))
vi.mock('@/features/editor/monaco/language-contributions', () => ({
  loadLanguageForPath: (path: string) => loadLanguageForPathFn(path),
}))

// defineMonacoTheme resolves colors off live CSS custom properties; jsdom has
// none, and real monaco's defineTheme rejects empty color strings. A built-in
// theme keeps the REAL editor-creation path intact without a CSS layer.
vi.mock('@/features/editor/monaco/define-theme', () => ({
  defineMonacoTheme: () => 'vs-dark',
}))

import { DiffMonacoEditor } from '@/features/editor/components/monaco-diff-editor'

describe('DiffMonacoEditor on-demand language load', () => {
  it('kicks off loadLanguageForPath for the buffer path on mount', () => {
    const store = createWorkspaceStore('diff-lang-ws')
    const bufferId = store.getState().bufferActions.openContent({
      type: 'editor',
      path: 'src/schema.graphql',
      name: 'schema.graphql',
      content: 'type Query { hello: String }',
    })

    render(
      createElement(
        WorkspaceStoreContext.Provider,
        { value: store },
        // readOnly + inactive = the git-diff-viewer configuration (also keeps
        // the LSP document lifecycle out of the test via shouldSkipLsp).
        createElement(DiffMonacoEditor, {
          bufferId,
          readOnly: true,
          isActiveSurface: false,
        }),
      ),
    )

    expect(loadLanguageForPathFn).toHaveBeenCalledWith('src/schema.graphql')
  })

  it('re-kicks the load when the shown buffer switches to a different file', () => {
    const store = createWorkspaceStore('diff-lang-ws-2')
    const first = store.getState().bufferActions.openContent({
      type: 'editor',
      path: 'src/main.go',
      name: 'main.go',
      content: 'package main',
    })
    const second = store.getState().bufferActions.openContent({
      type: 'editor',
      path: 'src/lib.rs',
      name: 'lib.rs',
      content: 'fn main() {}',
    })

    const view = render(
      createElement(
        WorkspaceStoreContext.Provider,
        { value: store },
        createElement(DiffMonacoEditor, {
          bufferId: first,
          readOnly: true,
          isActiveSurface: false,
        }),
      ),
    )
    expect(loadLanguageForPathFn).toHaveBeenCalledWith('src/main.go')

    view.rerender(
      createElement(
        WorkspaceStoreContext.Provider,
        { value: store },
        createElement(DiffMonacoEditor, {
          bufferId: second,
          readOnly: true,
          isActiveSurface: false,
        }),
      ),
    )
    expect(loadLanguageForPathFn).toHaveBeenCalledWith('src/lib.rs')
  })
})
