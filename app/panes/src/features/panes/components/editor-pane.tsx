import { lazy, Suspense, useState } from 'react'
import { ErrorBoundary } from '@/components/error-boundary'
import { Button } from '@/components/ui/button'
import { report } from '@/lib/errors/report'
import { useBufferById } from '@/features/workspace/stores/hooks/use-buffer-store'
import { useEditorHost } from '@/features/editor/host/use-editor-host'
import {
  loadEditorSurface,
  loadMarkdownEditorPane,
} from '@/features/panes/lib/editor-surface-loader'
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

const EditorSurface = lazy(() =>
  loadEditorSurface().then((m) => ({
    default: m.EditorSurface,
  })),
)

const MarkdownEditorPane = lazy(() =>
  loadMarkdownEditorPane().then((m) => ({
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

  const markdownView = useMarkdownViewStore((s) =>
    buffer ? (s.views[buffer.id] ?? 'rich') : 'rich',
  )

  const host = useEditorHost()
  const uri =
    buffer && isEditorContent(buffer) && buffer.path ? documentUri(documentRefOf(buffer)) : null
  const documentOpen = useDocumentOpen(uri)

  if (buffer && isEditorContent(buffer) && buffer.unavailable) {
    return <UnavailableDocument buffer={buffer} reason={buffer.unavailable} />
  }

  if (
    buffer &&
    isEditorContent(buffer) &&
    buffer.path &&
    isMarkdownPath(buffer.path) &&
    markdownView === 'rich'
  ) {
    if (!documentOpen) return null
    return (
      <ErrorBoundary
        fallback={
          <div className="flex h-full flex-1 items-center justify-center p-8 text-sm text-muted-foreground">
            Editor failed to load. Try closing and reopening this file.
          </div>
        }
      >
        <Suspense fallback={null}>
          <MarkdownEditorPane key={bufferId} paneId={paneId} bufferId={bufferId} />
        </Suspense>
      </ErrorBoundary>
    )
  }

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
    void retryLoad(buffer)
      .catch((err: unknown) => report(err, { action: 'Load the document' }))
      .finally(() => setRetrying(false))
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
