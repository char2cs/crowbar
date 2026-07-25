import { lazy, Suspense, useEffect, useState } from 'react'
import { EditorSurface } from '@/features/editor/components/editor-surface'
import { ErrorBoundary } from '@/components/error-boundary'
import { useBufferById } from '@/features/workspace/stores/hooks/use-buffer-store'
import { useWorkspaceStore } from '@/features/workspace/stores/workspace-context'
import { isEditorContent } from '@/features/panes/types/pane-content'
import { isMarkdownPath } from '@/features/editor/markdown/plate/is-markdown-path'
import { useMarkdownViewStore } from '@/features/editor/markdown/plate/markdown-view-store'

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

  // Lazy-Monaco seam (Task 4b): the workspace store constructs its Monaco-backed
  // EditorManager/ModelRegistry only on the first real editor need, so opening a
  // file is what pulls in `monaco-editor` — not cold launch. Arm it before we
  // mount EditorSurface (which reads `store.editorManager` synchronously). The
  // dynamic import resolves within this already-lazy pane chunk (monaco is loaded
  // alongside it), so `armed` flips on the same/next tick — no user-visible gap.
  // `armEditor` is idempotent, so a second pane opening an already-armed store
  // starts armed and renders immediately.
  const store = useWorkspaceStore()
  const [armed, setArmed] = useState(() => store.editorManager !== undefined)
  useEffect(() => {
    if (armed) return
    let cancelled = false
    void store
      .armEditor()
      .then(() => {
        if (!cancelled) setArmed(true)
      })
      // A failed adapter load leaves the pane blank (unarmed) rather than throwing
      // an unhandled rejection; a remount retries via the store's cleared promise.
      .catch(() => {})
    return () => {
      cancelled = true
    }
  }, [armed, store])

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

  // Markdown buffers in rich view route to Plate instead of Monaco. This must
  // come BEFORE the `!armed` gate below: rich mode never touches Monaco, so it
  // must not wait on Monaco's arming to render.
  if (buffer && isEditorContent(buffer) && isMarkdownPath(buffer.path) && markdownView === 'rich') {
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

  // Hold the surface back until the Monaco handles are armed. Rendering nothing
  // (rather than a spinner) avoids a flash: arming completes within the same lazy
  // chunk load that brought us here.
  if (!armed) return null

  return (
    <ErrorBoundary
      fallback={
        <div className="flex h-full flex-1 items-center justify-center p-8 text-sm text-muted-foreground">
          Editor failed to load. Try closing and reopening this file.
        </div>
      }
    >
      {/* Keyed by paneId so a buffer/tab switch swaps the model imperatively
          (via usePaneEditorController) instead of remounting the shell. */}
      <EditorSurface
        key={paneId}
        paneId={paneId}
        bufferId={bufferId}
        isActiveSurface={isActiveSurface}
        isPreview={isPreview}
        onPromote={onPromote}
        showToolbar={showToolbar}
        className={className}
      />
    </ErrorBoundary>
  )
}
