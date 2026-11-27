import { lazy, type ReactNode } from 'react'
import type {
  BrowserContent,
  EditorContent,
  PaneContent,
  TerminalContent,
} from '../types/pane-content'
import { NewTabView } from './new-tab-view'
import { windowPaneStore } from '../stores/window-pane-store'

const BranchReviewPane = lazy(() =>
  import('@/features/git/components/branch-review-pane').then((m) => ({
    default: m.BranchReviewPane,
  })),
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

export type RegisteredContent = Exclude<
  PaneContent,
  EditorContent | TerminalContent | BrowserContent
>

interface SurfaceContext {
  isActivePane: boolean
  paneId: string
  wsId: string | null
}

type Renderers = {
  [K in RegisteredContent['type']]: (
    content: Extract<RegisteredContent, { type: K }>,
    ctx: SurfaceContext,
  ) => ReactNode
}

const RENDERERS: Renderers = {
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
