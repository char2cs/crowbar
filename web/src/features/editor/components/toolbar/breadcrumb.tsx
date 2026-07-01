import type { ReactNode } from 'react'
import { Eye, MagnifyingGlass as Search } from '@phosphor-icons/react'
import { useShallow } from 'zustand/react/shallow'
import { EditorStatusActions } from '@/features/editor/components/toolbar/editor-status-actions'
import {
  useWorkspaceStoreContext,
  useWorkspaceStore,
} from '@/features/workspace/stores/workspace-context'
import { hasTextContent } from '@/features/panes/types/pane-content'
import { useUIState } from '@/features/window/stores/ui-state-store'
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
}: BreadcrumbProps = {}) {
  const workspaceStore = useWorkspaceStore()
  const resolvedBufferId = useWorkspaceStoreContext(
    (state) => bufferId ?? state.panes[paneId ?? state.activePaneId]?.activeBufferId ?? null,
  )
  const resolvedEditorViewKey =
    editorViewKey ?? (paneId && resolvedBufferId ? `${paneId}:${resolvedBufferId}` : editorViewKey)
  const activeBuffer = useWorkspaceStoreContext(
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
          }
        : null
    }),
  )
  const showBreadcrumbPath = useSettingsStore((state) => state.settings.coreFeatures.breadcrumbs)
  const { isFindVisible, setIsFindVisible } = useUIState(
    useShallow((state) => ({
      isFindVisible: state.isFindVisible,
      setIsFindVisible: state.setIsFindVisible,
    })),
  )
  const extensionActions = useExtensionActions()

  const handleSearchClick = () => {
    setIsFindVisible(!isFindVisible)
  }

  const isMarkdownFile = () => {
    if (!activeBuffer) return false
    const extension = activeBuffer.path.split('.').pop()?.toLowerCase()
    return extension === 'md' || extension === 'markdown'
  }

  const isHtmlFile = () => {
    if (!activeBuffer) return false
    const extension = activeBuffer.path.split('.').pop()?.toLowerCase()
    return extension === 'html' || extension === 'htm'
  }

  const isCsvFile = () => {
    if (!activeBuffer) return false
    const extension = activeBuffer.path.split('.').pop()?.toLowerCase()
    return extension === 'csv'
  }

  const handlePreviewClick = () => {
    const fullActiveBuffer = resolvedBufferId
      ? workspaceStore.getState().buffers.find((buffer) => buffer.id === resolvedBufferId)
      : null
    if (
      !fullActiveBuffer ||
      fullActiveBuffer.type === 'markdownPreview' ||
      fullActiveBuffer.type === 'htmlPreview' ||
      fullActiveBuffer.type === 'csvPreview'
    )
      return

    const previewPath = `${fullActiveBuffer.path}:preview`
    const previewName = `${fullActiveBuffer.name} (Preview)`

    const isMarkdown = isMarkdownFile()
    const isHtml = isHtmlFile()
    const isCsv = isCsvFile()

    const bufferContent = hasTextContent(fullActiveBuffer) ? fullActiveBuffer.content : ''

    if (isMarkdown) {
      workspaceStore.getState().bufferActions.openContent({
        type: 'markdownPreview',
        path: previewPath,
        name: previewName,
        content: bufferContent,
        sourceFilePath: fullActiveBuffer.path,
      })
    } else if (isHtml) {
      workspaceStore.getState().bufferActions.openContent({
        type: 'htmlPreview',
        path: previewPath,
        name: previewName,
        content: bufferContent,
        sourceFilePath: fullActiveBuffer.path,
      })
    } else if (isCsv) {
      workspaceStore.getState().bufferActions.openContent({
        type: 'csvPreview',
        path: previewPath,
        name: previewName,
        content: bufferContent,
        sourceFilePath: fullActiveBuffer.path,
      })
    }
  }

  const filePath = filePathOverride ?? activeBuffer?.path ?? ''
  const onSearchClick = handleSearchClick
  if (!filePath) return null
  const isLocalHistorySnapshot = filePath.startsWith('local-history://')

  const defaultActions =
    showDefaultActions && activeBuffer ? (
      <>
        {((isMarkdownFile() && activeBuffer.type !== 'markdownPreview') ||
          (isHtmlFile() && activeBuffer.type !== 'htmlPreview') ||
          (isCsvFile() && activeBuffer.type !== 'csvPreview')) && (
          <Button
            onClick={handlePreviewClick}
            variant="ghost"
            className="rounded text-muted-foreground"
            tooltip="Preview"
            tooltipSide="bottom"
            compact
          >
            <Eye />
          </Button>
        )}
        <Button
          onClick={onSearchClick}
          variant="ghost"
          className="rounded text-muted-foreground"
          tooltip="Find in file"
          commandId="workbench.showFind"
          tooltipSide="bottom"
          compact
        >
          <Search />
        </Button>
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
          {showPath && showBreadcrumbPath ? (
            <FilePathBreadcrumb
              filePath={filePath}
              interactive={interactive && !isLocalHistorySnapshot}
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
