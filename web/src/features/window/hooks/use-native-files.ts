import { useEffect, useMemo } from 'react'
import { useStore } from 'zustand'
import { openWorkspaceFile } from '@/features/files/lib/file-tree-handlers'
import { useFilteredFileTree } from '@/features/file-explorer/file-explorer/hooks/use-filtered-file-tree'
import type { FileTreeGitStatusDecoration } from '@/features/file-explorer/lib/file-tree-git-status'
import { useFileTreeStore } from '@/features/file-explorer/stores/file-explorer-tree-store'
import { getExplorerTargetPath } from '@/features/file-explorer/utils/file-explorer-tree-utils'
import type { FileEntry } from '@/features/file-system/types/app'
import { resolveOnscreenPaneForWorkspace } from '@/features/panes/lib/pane-chat-workspace'
import { windowPaneStore } from '@/features/panes/stores/window-pane-store'
import { preloadEditorFor } from '@/features/panes/lib/editor-surface-loader'
import { useChatBranchHeaderModel } from '@/features/tabs/hooks/use-chat-branch-header-model'
import { isImageUrl, projectIconUrl } from '@/features/window/hooks/use-native-sidebar'
import { useIconDataUris } from '@/lib/icon-data-uri'
import { useSettingsStore } from '@/features/settings/store'
import { useFocusedWorkspaceContextStore } from '@/features/window/stores/focused-workspace-context-store'
import { useMobileScreenStore } from '@/features/window/stores/mobile-screen-store'
import { useWorkspaceStoreById } from '@/features/workspace/stores/hooks/use-workspace-store-by-id'
import { isFileTreeLoading } from '@/features/workspace/stores/slices/file-tree-slice'
import {
  onNativeChromeIntent,
  setNativeFiles,
  type NativeFiles,
  type NativeFilesEntry,
} from '@/lib/native-chrome'

const EMPTY: NativeFiles = {
  header: null,
  loading: false,
  files: [],
  expanded: [],
  activePath: null,
  compactFolders: false,
}

function toEntries(
  files: FileEntry[],
  decorate: (file: FileEntry) => FileTreeGitStatusDecoration | null,
): NativeFilesEntry[] {
  return files.flatMap((file) =>
    file.isNewItem || file.isEditing
      ? []
      : [
          {
            name: file.name,
            path: file.path,
            isDir: file.isDir ?? false,
            isSymlink: file.isSymlink,
            ignored: file.ignored,
            git: decorate(file),
            children: file.children && toEntries(file.children, decorate),
          },
        ],
  )
}

/** The chat's header with its icon as data, which the drawer page cannot fetch for itself. */
function useDrawerHeader(chatId: string, wsId: string): NativeFiles['header'] {
  const model = useChatBranchHeaderModel(chatId, wsId)
  const { lead } = model
  const url =
    lead?.kind === 'repo'
      ? lead.repo.avatarURL
      : lead?.kind === 'project'
        ? projectIconUrl(lead.project)
        : undefined
  const icons = useIconDataUris(isImageUrl(url) ? [url] : [])
  return useMemo(() => {
    if (!lead || !isImageUrl(url)) return model
    const data = icons.get(url) ?? undefined
    return {
      ...model,
      lead:
        lead.kind === 'repo'
          ? { ...lead, repo: { ...lead.repo, avatarURL: data } }
          : { ...lead, project: { ...lead.project, avatarUrl: data } },
    }
  }, [model, lead, url, icons])
}

function useActiveFilePath(): string | null {
  const buffer = useStore(windowPaneStore, (s) => {
    const tabId = s.paneActions.getActivePane()?.activeEditorTabId
    return s.buffers.find((b) => b.id === tabId) ?? null
  })
  return getExplorerTargetPath(buffer) ?? null
}

/**
 * Feeds the phone's file drawer with `wsId`'s tree, filtered and git-marked as the desktop
 * explorer draws it, and carries out what the drawer asks: fold a folder, open a file, close.
 */
export function useNativeFiles(wsId: string, chatId: string) {
  const header = useDrawerHeader(chatId, wsId)
  const rootPath = useFocusedWorkspaceContextStore((s) => s.rootPath)
  const files = useWorkspaceStoreById(wsId, (s) => s.files)
  const loading = useWorkspaceStoreById(wsId, isFileTreeLoading)
  const expanded = useFileTreeStore((s) => s.getExpandedPaths(wsId))
  const compactFolders = useSettingsStore((s) => s.settings.compactFoldersInFileTree)
  const activePath = useActiveFilePath()
  const { filteredFiles, getGitStatusDecoration } = useFilteredFileTree({
    workspaceId: wsId,
    files,
    rootFolderPath: rootPath,
  })

  const snapshot = useMemo<NativeFiles>(
    () => ({
      header,
      loading,
      files: toEntries(filteredFiles, getGitStatusDecoration),
      expanded: [...expanded],
      activePath,
      compactFolders,
    }),
    [header, loading, filteredFiles, getGitStatusDecoration, expanded, activePath, compactFolders],
  )
  useEffect(() => {
    setNativeFiles(snapshot)
  }, [snapshot])
  useEffect(
    () => () => {
      setNativeFiles(EMPTY)
    },
    [],
  )

  useEffect(() => {
    const close = () => useMobileScreenStore.getState().setFilesOpen(false)
    return onNativeChromeIntent((intent) => {
      if (intent.type === 'toggleDir') useFileTreeStore.getState().toggleFolder(wsId, intent.path)
      else if (intent.type === 'openFile') {
        const paneId = resolveOnscreenPaneForWorkspace(wsId) ?? undefined
        // The IDE stays behind the drawer until the file's tab exists and its editor is loaded,
        // so the pane is never seen empty or blank while it loads.
        void Promise.all([
          openWorkspaceFile(wsId, intent.path, { preview: true, paneId }),
          preloadEditorFor(intent.path).catch(() => undefined),
        ]).then(close)
      }
    })
  }, [wsId])
}
