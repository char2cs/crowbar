import { EditorSurface } from '@/features/editor/components/editor-surface'
import { ErrorBoundary } from '@/components/error-boundary'
import { useBufferById } from '@/features/workspace/stores/hooks/use-buffer-store'
import { isEditorContent } from '@/features/panes/types/pane-content'

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
