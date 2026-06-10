import CodeEditor from '@/features/editor/components/code-editor'
import { ErrorBoundary } from '@/components/ErrorBoundary'

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
  return (
    <ErrorBoundary
      fallback={
        <div className="flex h-full flex-1 items-center justify-center p-8 text-sm text-muted-foreground">
          Editor failed to load. Try closing and reopening this file.
        </div>
      }
    >
      <CodeEditor
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
