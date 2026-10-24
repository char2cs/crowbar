import type { ReactNode } from 'react'
import { useStore } from 'zustand'
import { Eye, MagnifyingGlass as Search } from '@phosphor-icons/react'
import { useShallow } from 'zustand/react/shallow'
import { EditorStatusActions } from '@/features/editor/components/toolbar/editor-status-actions'
import { windowPaneStore } from '@/features/panes/stores/window-pane-store'
import { isEditorContent } from '@/features/panes/types/pane-content'
import { documentRefOf } from '@/features/editor/documents/document-ref'
import { useExtensionActions } from '@/extensions/ui/hooks/use-extension-actions'
import { ExtensionToolbarAction } from '@/extensions/ui/components/extension-toolbar-action'
import { useSettingsStore } from '@/features/settings/store'
import { Button } from '@/components/ui/button'
import { FilePathBreadcrumb } from './file-path-breadcrumb'

export interface BreadcrumbProps {
  /**
   * Resolve the active buffer for THIS pane (self-subscribing). When omitted the
   * breadcrumb falls back to `bufferId`, then to the globally active pane.
   */
  paneId?: string
  bufferId?: string
  editorViewKey?: string | null
  filePathOverride?: string
  rightContent?: ReactNode
  extraLeftContent?: ReactNode
  showDefaultActions?: boolean
  interactive?: boolean
  showPath?: boolean
  /** Opens the editor's find widget; the search button shows only when set. */
  onFind?: () => void
}

export default function Breadcrumb({
  paneId,
  bufferId,
  editorViewKey,
  filePathOverride,
  rightContent,
  extraLeftContent,
  showDefaultActions = true,
  interactive = true,
  showPath = true,
  onFind,
}: BreadcrumbProps = {}) {
  const resolvedBufferId = useStore(
    windowPaneStore,
    (state) => bufferId ?? state.panes[paneId ?? state.activePaneId]?.activeEditorTabId ?? null,
  )
  const resolvedEditorViewKey =
    editorViewKey ?? (paneId && resolvedBufferId ? `${paneId}:${resolvedBufferId}` : editorViewKey)
  const activeBuffer = useStore(
    windowPaneStore,
    useShallow((state) => {
      const buffer = resolvedBufferId
        ? state.buffers.find((candidate) => candidate.id === resolvedBufferId)
        : null
      return buffer
        ? {
            id: buffer.id,
            path: buffer.path,
            name: buffer.name,
            type: buffer.type,
            // A host document has no workspace to list or open its segments in.
            workspaceId:
              isEditorContent(buffer) && buffer.authority === 'host' ? null : buffer.workspaceId,
          }
        : null
    }),
  )
  const showBreadcrumbPath = useSettingsStore((state) => state.settings.coreFeatures.breadcrumbs)
  const extensionActions = useExtensionActions()

  const isMarkdownFile = () => {
    if (!activeBuffer || !activeBuffer.path) return false
    const extension = activeBuffer.path.split('.').pop()?.toLowerCase()
    return extension === 'md' || extension === 'markdown'
  }

  const isHtmlFile = () => {
    if (!activeBuffer || !activeBuffer.path) return false
    const extension = activeBuffer.path.split('.').pop()?.toLowerCase()
    return extension === 'html' || extension === 'htm'
  }

  const handlePreviewClick = () => {
    const fullActiveBuffer = resolvedBufferId
      ? windowPaneStore.getState().buffers.find((buffer) => buffer.id === resolvedBufferId)
      : null
    if (!fullActiveBuffer || !isEditorContent(fullActiveBuffer) || !fullActiveBuffer.path) return

    // The preview belongs to the source document, whichever workspace is active.
    const preview = {
      name: `${fullActiveBuffer.name} (Preview)`,
      content: fullActiveBuffer.content,
      source: documentRefOf(fullActiveBuffer),
    }
    const { openContent } = windowPaneStore.getState().bufferActions
    if (isMarkdownFile()) openContent({ type: 'markdownPreview', ...preview }, { paneId })
    else if (isHtmlFile()) openContent({ type: 'htmlPreview', ...preview }, { paneId })
  }

  const filePath = filePathOverride ?? activeBuffer?.path ?? ''
  if (!filePath) return null

  const defaultActions =
    showDefaultActions && activeBuffer ? (
      <>
        {activeBuffer.type === 'editor' && (isMarkdownFile() || isHtmlFile()) && (
          <Button
            onClick={handlePreviewClick}
            variant="ghost"
            className="rounded text-muted-foreground"
            aria-label="Preview"
            tooltip="Preview"
            tooltipSide="bottom"
            compact
          >
            <Eye />
          </Button>
        )}
        {onFind && (
          <Button
            onClick={onFind}
            variant="ghost"
            className="rounded text-muted-foreground"
            tooltip="Find in file"
            tooltipSide="bottom"
            compact
          >
            <Search />
          </Button>
        )}
        <div className="mx-1 h-3.5 w-px bg-border/70" />
        <EditorStatusActions
          bufferId={resolvedBufferId ?? undefined}
          editorViewKey={resolvedEditorViewKey}
        />
      </>
    ) : null

  return (
    <>
      <div className="flex min-h-7 select-none items-center justify-between bg-terniary-bg px-3 py-1">
        <div className="ui-font flex min-w-0 items-center gap-2 text-muted-foreground ui-text-xs">
          {showPath && showBreadcrumbPath && activeBuffer ? (
            <FilePathBreadcrumb
              filePath={filePath}
              workspaceId={activeBuffer.workspaceId}
              interactive={interactive}
            />
          ) : null}
          {extensionActions.left.map((action) => (
            <ExtensionToolbarAction key={action.id} action={action} />
          ))}
          {extraLeftContent}
        </div>
        <div className="flex items-center gap-1">
          {defaultActions}
          {defaultActions && rightContent ? <div className="mx-1 h-3.5 w-px bg-border/70" /> : null}
          {rightContent}
          {extensionActions.right.map((action) => (
            <ExtensionToolbarAction key={action.id} action={action} />
          ))}
        </div>
      </div>
    </>
  )
}
