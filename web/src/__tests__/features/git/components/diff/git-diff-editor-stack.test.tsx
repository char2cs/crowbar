import { act, fireEvent, render, screen } from '@testing-library/react'
import { createElement, Profiler, type ReactNode } from 'react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import type { EditorContent } from '@/features/panes/types/pane-content'
import { TooltipProvider } from '@/components/ui/tooltip'
import { WorkspaceStoreContext } from '@/features/workspace/stores/workspace-context'
import { createWorkspaceStore } from '@/features/workspace/stores/workspace-store'
import type { MultiFileDiff } from '@/features/git/types/git-diff-types'
import type { GitDiff } from '@/features/git/types/git-types'

// Mocks must be hoisted before the component import that pulls them in.
vi.mock('@/features/git/api/identity-api', () => ({
  getIdentity: vi.fn().mockResolvedValue({ login: 'tester', displayName: 'Tester', avatarUrl: '' }),
}))

// CodeEditor is the real Monaco-backed surface — stubbed to a passive marker
// so these tests exercise buffer REGISTRATION (the workspace store) without
// paying for (or depending on) an actual Monaco mount.
vi.mock('@/features/editor/components/code-editor', () => ({
  default: ({ bufferId }: { bufferId: string }) =>
    createElement('div', { 'data-testid': 'code-editor', 'data-buffer-id': bufferId }),
}))

// Breadcrumb pulls in the extension toolbar + settings store, none of which
// this suite cares about — stub it down to rendering the slots the stack
// actually wires content through (the view-mode toggle lives in rightContent).
vi.mock('@/features/editor/components/toolbar/breadcrumb', () => ({
  default: ({
    extraLeftContent,
    rightContent,
  }: {
    extraLeftContent?: ReactNode
    rightContent?: ReactNode
  }) => createElement('div', { 'data-testid': 'breadcrumb-stub' }, extraLeftContent, rightContent),
}))

// jsdom has no IntersectionObserver; LazyDiffSectionBody uses one to decide
// when to mount a file section's real content. Fire it synchronously so the
// (auto-expanded) first file section actually mounts.
class ImmediateIntersectionObserver implements IntersectionObserver {
  readonly root: Element | Document | null = null
  readonly rootMargin: string = ''
  readonly thresholds: ReadonlyArray<number> = []
  private callback: IntersectionObserverCallback
  constructor(callback: IntersectionObserverCallback) {
    this.callback = callback
  }
  observe(target: Element) {
    this.callback(
      [{ isIntersecting: true, target } as IntersectionObserverEntry],
      this as unknown as IntersectionObserver,
    )
  }
  unobserve() {}
  disconnect() {}
  takeRecords(): IntersectionObserverEntry[] {
    return []
  }
}

// The virtualizer measures real DOM layout (all zero under jsdom) — replace it
// with a deterministic stand-in that renders every item, so section mounting
// (and therefore buffer registration) doesn't depend on jsdom layout numbers.
vi.mock('@tanstack/react-virtual', () => ({
  useVirtualizer: (options: { count: number }) => ({
    getVirtualItems: () =>
      Array.from({ length: options.count }, (_, index) => ({ index, start: 0, key: index })),
    getTotalSize: () => 0,
    scrollToIndex: () => {},
    measureElement: () => {},
  }),
}))

import GitDiffEditorStack from '@/features/git/components/diff/git-diff-editor-stack'

beforeEach(() => {
  vi.stubGlobal('IntersectionObserver', ImmediateIntersectionObserver)
})

afterEach(() => {
  vi.unstubAllGlobals()
})

function makeFileDiff(path: string): GitDiff {
  return {
    file_path: path,
    is_new: false,
    is_deleted: false,
    is_renamed: false,
    lines: [
      { line_type: 'context', content: 'const x = 1', old_line_number: 1, new_line_number: 1 },
      { line_type: 'added', content: 'const y = 2', new_line_number: 2 },
    ],
  }
}

function makeMultiDiff(overrides: Partial<MultiFileDiff> = {}): MultiFileDiff {
  return {
    commitHash: 'abc123',
    files: [makeFileDiff('src/foo.ts')],
    totalFiles: 1,
    totalAdditions: 1,
    totalDeletions: 0,
    fileKeys: ['src/foo.ts:0'],
    ...overrides,
  }
}

function diffEditorBuffers(buffers: Array<{ id: string }>) {
  return buffers.filter((b) => b.id.startsWith('diff_editor_'))
}

// The (always-called, per Rules of Hooks) review-comment layer eagerly resolves
// the workspace identity on mount regardless of whether comments are enabled.
// Flushing one microtask tick lets that promise settle before assertions, so it
// doesn't land as an unwrapped act() warning attributed to a later test.
async function flush() {
  await act(async () => {
    await Promise.resolve()
  })
}

async function renderStack(
  store: ReturnType<typeof createWorkspaceStore>,
  multiDiff: MultiFileDiff,
) {
  const utils = render(
    createElement(
      TooltipProvider,
      null,
      createElement(
        WorkspaceStoreContext.Provider,
        { value: store },
        createElement(GitDiffEditorStack, { multiDiff }),
      ),
    ),
  )
  await flush()
  return utils
}

describe('GitDiffEditorStack split-view buffer registration', () => {
  // Regression for Task 9 / P2d: split serialization + the left/right buffers
  // used to be registered unconditionally, tripling the Monaco buffer/model
  // churn for every file even though unified is the default view.
  it('registers exactly one diff buffer in unified view', async () => {
    const store = createWorkspaceStore('w1')
    await renderStack(store, makeMultiDiff())

    expect(diffEditorBuffers(store.getState().buffers)).toHaveLength(1)
  })

  it('registers three diff buffers (unified + left + right) once split view is on', async () => {
    const store = createWorkspaceStore('w1')
    await renderStack(store, makeMultiDiff())

    fireEvent.click(screen.getByRole('button', { name: 'Split view' }))

    expect(diffEditorBuffers(store.getState().buffers)).toHaveLength(3)
  })

  it('registers the missing split buffers on the spot when toggled mid-session, and drops them when toggled back', async () => {
    const store = createWorkspaceStore('w1')
    await renderStack(store, makeMultiDiff())
    expect(diffEditorBuffers(store.getState().buffers)).toHaveLength(1)

    fireEvent.click(screen.getByRole('button', { name: 'Split view' }))
    expect(diffEditorBuffers(store.getState().buffers)).toHaveLength(3)

    fireEvent.click(screen.getByRole('button', { name: 'Unified view' }))
    expect(diffEditorBuffers(store.getState().buffers)).toHaveLength(1)
  })
})

describe('GitDiffEditorStack render isolation', () => {
  // Regression for Task 9 / P2d: the whole `buffers` array was subscribed in
  // render just to compute `activeBuffer` inside a callback — scrolling a huge
  // diff (buffer mount/unmount via useDiffEditorBuffer) re-rendered the entire
  // stack (tab bar included) on every scroll tick.
  it('does not re-render when an unrelated buffer is added to the workspace store', async () => {
    const store = createWorkspaceStore('w1')
    let renderCount = 0
    const multiDiff = makeMultiDiff({
      commitHash: 'working-tree',
      files: [],
      totalFiles: 0,
      fileKeys: [],
      isLoading: false,
    })

    render(
      createElement(
        TooltipProvider,
        null,
        createElement(
          WorkspaceStoreContext.Provider,
          { value: store },
          createElement(
            Profiler,
            {
              id: 'stack',
              onRender: () => {
                renderCount += 1
              },
            },
            createElement(GitDiffEditorStack, { multiDiff }),
          ),
        ),
      ),
    )
    await flush()

    // A self-correcting effect (expandedFiles resync, unrelated to this task)
    // re-renders once more right after mount regardless of the fix under test —
    // capture that settled count as the baseline instead of assuming 1.
    const settledRenderCount = renderCount
    expect(settledRenderCount).toBeGreaterThan(0)

    const unrelatedBuffer: EditorContent = {
      id: 'unrelated-buffer',
      type: 'editor',
      path: '/tmp/unrelated.ts',
      name: 'unrelated.ts',
      content: '',
      savedContent: '',
      isDirty: false,
      isVirtual: false,
      isPreview: false,
      isPinned: false,
      isActive: false,
      language: 'typescript',
      tokens: [],
    }

    act(() => {
      store.setState({ buffers: [...store.getState().buffers, unrelatedBuffer] })
    })

    expect(renderCount).toBe(settledRenderCount)
  })
})
