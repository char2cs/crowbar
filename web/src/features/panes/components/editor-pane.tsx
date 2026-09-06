import { lazy, Suspense, useEffect, useState } from 'react'
import { EditorSurface } from '@/features/editor/components/editor-surface'
import { ErrorBoundary } from '@/components/error-boundary'
import { useBufferById } from '@/features/workspace/stores/hooks/use-buffer-store'
import { useWorkspaceStoreContext } from '@/features/workspace/stores/workspace-context'
import { getWorkspaceStore } from '@/features/workspace/stores/workspace-store-registry'
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
  // mount EditorSurface (which reads the manager synchronously). The dynamic
  // import resolves within this already-lazy pane chunk (monaco is loaded
  // alongside it), so `armed` flips on the same/next tick — no user-visible gap.
  // `armEditor` is idempotent, so a second pane opening an already-armed store
  // starts armed and renders immediately.
  //
  // Resolved by the BUFFER's own workspace (buffer.workspaceId) whenever that
  // workspace already has a registered store — NOT the ambient
  // WorkspaceStoreContext: WorkspaceHost keeps every retained WorkspaceView
  // mounted at once for keep-alive, each rendering the same window-level pane
  // tree under a DIFFERENT ambient context. Arming (and later mounting
  // EditorSurface's Monaco widget against) the ambient workspace's
  // EditorManager instead of the buffer's own would let a wrong-ambient
  // hidden copy create a second, leaked Monaco model/widget under a manager
  // the buffer's own `closeBuffer` cleanup — scoped to buf.workspaceId, see
  // buffer-slice.ts's `editorManagerFor` — never visits.
  //
  // Falls back to the ambient workspace only when the buffer's own workspace
  // has NO store yet — e.g. a buffer opened by a chat the user hasn't
  // navigated into this session, so nothing ever called
  // `getOrCreateWorkspaceStore` for it. Minting one here instead (rather than
  // falling back) would register a store WorkspaceHost never agreed to retain
  // and will never destroy (see getWorkspaceStore's own doc) — a worse,
  // permanent leak than the wrong-ambient-copy one this fix targets. Live-
  // verified: without this fallback, such a buffer's tab renders permanently
  // blank instead of merely wrong-scoped. The fallback never fires for the
  // keep-alive-retention case above, since a retained workspace's store
  // already exists.
  const ambientWorkspaceId = useWorkspaceStoreContext((s) => s.workspaceId)
  const workspaceId =
    buffer && getWorkspaceStore(buffer.workspaceId) ? buffer.workspaceId : ambientWorkspaceId
  const workspaceStore = getWorkspaceStore(workspaceId)
  const [armed, setArmed] = useState(() => workspaceStore?.editorManager !== undefined)
  useEffect(() => {
    if (armed || !workspaceStore) return
    let cancelled = false
    void workspaceStore
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
  }, [armed, workspaceStore])

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

  // Hold the surface back until the Monaco handles are armed (also covers the
  // buffer not having resolved yet — `workspaceStore` above is undefined
  // without one, so `armed` can never flip true). Rendering nothing (rather
  // than a spinner) avoids a flash: arming completes within the same lazy
  // chunk load that brought us here.
  if (!armed || !buffer) return null

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
        workspaceId={workspaceId}
        isActiveSurface={isActiveSurface}
        isPreview={isPreview}
        onPromote={onPromote}
        showToolbar={showToolbar}
        className={className}
      />
    </ErrorBoundary>
  )
}
