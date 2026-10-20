import { lazy, Suspense } from 'react'
import { ErrorBoundary } from '@/components/error-boundary'
import { useBufferById } from '@/features/workspace/stores/hooks/use-buffer-store'
import { useEditorHost } from '@/features/editor/host/use-editor-host'
import { isEditorContent } from '@/features/panes/types/pane-content'
import { isMarkdownPath } from '@/features/editor/markdown/plate/is-markdown-path'
import { useMarkdownViewStore } from '@/features/editor/markdown/plate/markdown-view-store'

// Lazy so Monaco (3.5 MB, half of all boot JS) loads when an editor pane first
// renders, not at app boot.
const EditorSurface = lazy(() =>
  import('@/features/editor/components/editor-surface').then((m) => ({
    default: m.EditorSurface,
  })),
)

// Lazy so Plate (and its dependency graph) stays out of the base pane chunk —
// only buffers that actually route to the rich surface pull it in.
const MarkdownEditorPane = lazy(() =>
  import('@/features/editor/markdown/plate/markdown-editor-pane').then((m) => ({
    default: m.MarkdownEditorPane,
  })),
)

interface EditorPaneProps {
  paneId: string
  bufferId: string
  isActiveSurface: boolean
  isPreview: boolean
  onPromote: () => void
  showToolbar?: boolean
  className?: string
}

export function EditorPane({
  paneId,
  bufferId,
  isActiveSurface,
  isPreview,
  onPromote,
  showToolbar,
  className,
}: EditorPaneProps) {
  const buffer = useBufferById(bufferId)

  // Declared up with the other hooks (before any early return) — zustand
  // compares the selected *result*, so this inline selector is stable even
  // though it's a fresh arrow each render.
  const markdownView = useMarkdownViewStore((s) =>
    buffer ? (s.views[buffer.id] ?? 'rich') : 'rich',
  )

  const host = useEditorHost()

  // BUG-001: the file backing a restored buffer no longer exists on disk.
  // Render a terminal placeholder instead of an editor — there is no content
  // to edit and re-fetching the dead path would only repeat the 404.
  if (buffer && isEditorContent(buffer) && buffer.fileMissing) {
    return (
      <div className="flex h-full flex-1 flex-col items-center justify-center gap-1 p-8 text-center">
        <span className="text-sm font-medium text-foreground">File not found</span>
        <span className="text-xs text-muted-foreground">
          {buffer.path} no longer exists on disk. Close this tab, or restore the file to reload it.
        </span>
      </div>
    )
  }

  // Markdown buffers in rich view route to Plate instead of Monaco. Ahead of
  // the host gate: rich mode never touches Monaco, so it never waits on it.
  if (
    buffer &&
    isEditorContent(buffer) &&
    buffer.path &&
    isMarkdownPath(buffer.path) &&
    markdownView === 'rich'
  ) {
    return (
      // M8: the lazy chunk can fail to load (offline, a stale asset hash after
      // a deploy) and a rejected `lazy()` throws during render — Suspense only
      // handles the pending state, not the rejection. Without a boundary here
      // that escapes the pane and takes the whole workspace down.
      <ErrorBoundary
        fallback={
          <div className="flex h-full flex-1 items-center justify-center p-8 text-sm text-muted-foreground">
            Editor failed to load. Try closing and reopening this file.
          </div>
        }
      >
        <Suspense fallback={null}>
          {/* C1: the pane tree renders the active buffer WITHOUT a key, so a
              `.md` -> `.md` tab switch arrives here as a prop update. The rich
              editor parses its document once per mount, so it must be keyed by
              buffer — otherwise file A's document stays live while `bufferId`
              (and the flush target) moves to B, and the next edit writes A's
              whole text into B. */}
          <MarkdownEditorPane
            key={bufferId}
            paneId={paneId}
            bufferId={bufferId}
            isPreview={isPreview}
            onPromote={onPromote}
          />
        </Suspense>
      </ErrorBoundary>
    )
  }

  // Nothing (not a spinner) until Monaco has loaded: it loads alongside the
  // surface's own lazy chunk, so a spinner would only flash.
  if (!host || !buffer) return null

  return (
    <ErrorBoundary
      fallback={
        <div className="flex h-full flex-1 items-center justify-center p-8 text-sm text-muted-foreground">
          Editor failed to load. Try closing and reopening this file.
        </div>
      }
    >
      <Suspense fallback={null}>
        {/* Keyed by paneId so a buffer/tab switch swaps the model imperatively
            (via usePaneEditorController) instead of remounting the shell. */}
        <EditorSurface
          key={paneId}
          paneId={paneId}
          bufferId={bufferId}
          host={host}
          isActiveSurface={isActiveSurface}
          isPreview={isPreview}
          onPromote={onPromote}
          showToolbar={showToolbar}
          className={className}
        />
      </Suspense>
    </ErrorBoundary>
  )
}
