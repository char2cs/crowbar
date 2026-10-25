import { lazy, Suspense, useState } from 'react'
import { ErrorBoundary } from '@/components/error-boundary'
import { Button } from '@/components/ui/button'
import { bestEffort } from '@/lib/best-effort'
import { useBufferById } from '@/features/workspace/stores/hooks/use-buffer-store'
import { useEditorHost } from '@/features/editor/host/use-editor-host'
import {
  isEditorContent,
  type DocumentUnavailable,
  type EditorContent,
} from '@/features/panes/types/pane-content'
import { documentRefOf, documentUri } from '@/features/editor/documents/document-ref'
import { retryLoad } from '@/features/editor/documents/open-document'
import { useDocumentOpen } from '@/features/editor/documents/use-document-open'
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
  showToolbar?: boolean
  className?: string
}

export function EditorPane({
  paneId,
  bufferId,
  isActiveSurface,
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
  const uri =
    buffer && isEditorContent(buffer) && buffer.path ? documentUri(documentRefOf(buffer)) : null
  const documentOpen = useDocumentOpen(uri)

  // The document could not be loaded: an empty editor would read as an empty file.
  if (buffer && isEditorContent(buffer) && buffer.unavailable) {
    return <UnavailableDocument buffer={buffer} reason={buffer.unavailable} />
  }

  // Markdown buffers in rich view route to Plate instead of Monaco's editor.
  // Plate parses its document once, so it waits for the document's text.
  if (
    buffer &&
    isEditorContent(buffer) &&
    buffer.path &&
    isMarkdownPath(buffer.path) &&
    markdownView === 'rich'
  ) {
    if (!documentOpen) return null
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
          <MarkdownEditorPane key={bufferId} paneId={paneId} bufferId={bufferId} />
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
          showToolbar={showToolbar}
          className={className}
        />
      </Suspense>
    </ErrorBoundary>
  )
}

/** Why a tab's document could not be loaded, with a retry that runs one at a time. */
function UnavailableDocument({
  buffer,
  reason,
}: {
  buffer: EditorContent
  reason: DocumentUnavailable
}) {
  const [retrying, setRetrying] = useState(false)
  const retry = () => {
    setRetrying(true)
    bestEffort(
      retryLoad(buffer).finally(() => setRetrying(false)),
      'retry document',
    )
  }
  return (
    <div className="flex h-full flex-1 flex-col items-center justify-center gap-1 p-8 text-center">
      <span className="text-sm font-medium text-foreground">{reason.title}</span>
      <span className="text-xs text-muted-foreground">{reason.message}</span>
      <Button variant="outline" size="xs" className="mt-2" loading={retrying} onClick={retry}>
        Try again
      </Button>
    </div>
  )
}
