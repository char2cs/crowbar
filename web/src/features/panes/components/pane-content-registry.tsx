import { lazy, type ReactNode } from 'react'
import type {
  BrowserContent,
  EditorContent,
  PaneContent,
  TerminalContent,
} from '../types/pane-content'
import { NewTabView } from './new-tab-view'
import { windowPaneStore } from '../stores/window-pane-store'

// Lazy so each surface's heavy dependencies (git diff, markdown parser, CSV
// grid) stay out of the main chunk until a tab of that type is opened.
const BranchReviewPane = lazy(() =>
  import('@/features/git/components/branch-review-pane').then((m) => ({
    default: m.BranchReviewPane,
  })),
)
const CommitDiffPane = lazy(() =>
  import('./commit-diff-pane').then((m) => ({ default: m.CommitDiffPane })),
)
const MarkdownPreview = lazy(() =>
  import('@/features/editor/markdown/markdown-preview').then((m) => ({
    default: m.MarkdownPreview,
  })),
)
const HtmlPreview = lazy(() =>
  import('@/features/editor/components/html/html-preview').then((m) => ({
    default: m.HtmlPreview,
  })),
)

/** A tab a pane renders on demand. Editors (portaled), terminals (PTY) and browsers (webview)
 *  are kept mounted by the pane itself and never reach this registry. */
export type RegisteredContent = Exclude<
  PaneContent,
  EditorContent | TerminalContent | BrowserContent
>

interface SurfaceContext {
  isActivePane: boolean
  paneId: string
  /** The pane's own chat workspace; null until it resolves. */
  wsId: string | null
}

type Renderers = {
  [K in RegisteredContent['type']]: (
    content: Extract<RegisteredContent, { type: K }>,
    ctx: SurfaceContext,
  ) => ReactNode
}

// One entry per tab type a pane can show; the mapped type makes a missing one a type error.
// A diff's `wsId` is the buffer's own workspace, never the ambient one.
const RENDERERS: Renderers = {
  commitDiff: (c, { isActivePane }) => (
    <CommitDiffPane sha={c.sha} wsId={c.wsId} isActivePane={isActivePane} />
  ),
  branchReview: (c, { isActivePane }) => (
    <BranchReviewPane
      view={c.view}
      onViewChange={(view) =>
        windowPaneStore.getState().bufferActions.setBranchReviewView(c.id, view)
      }
      wsId={c.wsId}
      isActivePane={isActivePane}
    />
  ),
  markdownPreview: (c) => <MarkdownPreview bufferId={c.id} />,
  htmlPreview: (c) => <HtmlPreview bufferId={c.id} />,
  newTabPage: (c, { paneId, wsId }) => (
    <NewTabView paneId={paneId} wsId={wsId} replaceTabId={c.id} />
  ),
}

export function renderPaneContent(content: RegisteredContent, ctx: SurfaceContext): ReactNode {
  const render = RENDERERS[content.type] as (c: RegisteredContent, x: SurfaceContext) => ReactNode
  return render(content, ctx)
}
