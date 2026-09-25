import { useCallback, useState } from 'react'
import { readDirectory } from '@/features/file-system/controllers/platform'
import type { FileEntry } from '@/features/file-system/types/app'
import {
  collectLoadedFilesInDirectory,
  collectLocalFilesInDirectory,
  getPathBaseName,
  OPEN_ALL_CONFIRM_THRESHOLD,
} from '../lib/open-all'
import type { FileExplorerDialogs } from '../components/file-explorer-dialogs'

type OpenHandler = (path: string, isDir: boolean) => void | Promise<void>

/**
 * The explorer's modal flows — an alert, the delete confirmation and "Open
 * All" (confirmed past a threshold) — as state plus the props that render them.
 */
export function useFileExplorerDialogs({
  filteredFiles,
  isVisible,
  rootFolderPath,
  onFileSelect,
  onFileOpen,
  onDeletePath,
  updateActivePath,
}: {
  filteredFiles: FileEntry[]
  isVisible: Parameters<typeof collectLocalFilesInDirectory>[2]
  rootFolderPath?: string
  onFileSelect: OpenHandler
  onFileOpen?: OpenHandler
  onDeletePath?: (path: string, isDir: boolean) => void
  updateActivePath?: (path: string) => void
}) {
  const [deleteCandidate, setDeleteCandidate] = useState<{ path: string; isDir: boolean } | null>(
    null,
  )
  const [alertDialog, setAlertDialog] = useState<{ title: string; message: string } | null>(null)
  const [openAllFilesDialog, setOpenAllFilesDialog] = useState<{ filePaths: string[] } | null>(null)
  const [isDeletingPath, setIsDeletingPath] = useState(false)
  const [isOpeningAllFiles, setIsOpeningAllFiles] = useState(false)

  const showAlertDialog = useCallback((title: string, message: string) => {
    setAlertDialog({ title, message })
  }, [])

  const openFilePathsInTabs = useCallback(
    async (filePaths: string[]) => {
      const open = onFileOpen ?? onFileSelect
      for (const filePath of filePaths) {
        // react-doctor-disable-next-line async-await-in-loop -- kept sequential: each open reads the pane's current tab list via getState() and appends, so concurrent opens could race on that read-modify-write and land tabs out of drop order. Rare (multi-file drag-drop), not a hot path.
        await Promise.resolve(open(filePath, false))
      }
      updateActivePath?.(filePaths[filePaths.length - 1])
    },
    [onFileOpen, onFileSelect, updateActivePath],
  )

  const openAllFilesInDirectory = useCallback(
    async (directoryPath: string) => {
      let filePaths: string[]
      try {
        filePaths = await collectLocalFilesInDirectory(directoryPath, readDirectory, isVisible)
      } catch (error) {
        console.error('Failed to scan directory for Open All, falling back to loaded tree:', error)
        filePaths = collectLoadedFilesInDirectory(filteredFiles, directoryPath, rootFolderPath)
      }

      const uniqueFilePaths = Array.from(new Set(filePaths))
      if (uniqueFilePaths.length === 0) return
      if (uniqueFilePaths.length > OPEN_ALL_CONFIRM_THRESHOLD) {
        setOpenAllFilesDialog({ filePaths: uniqueFilePaths })
        return
      }
      await openFilePathsInTabs(uniqueFilePaths)
    },
    [filteredFiles, isVisible, openFilePathsInTabs, rootFolderPath],
  )

  const confirmOpenAllFiles = useCallback(async () => {
    if (!openAllFilesDialog) return
    setIsOpeningAllFiles(true)
    try {
      await openFilePathsInTabs(openAllFilesDialog.filePaths)
      setOpenAllFilesDialog(null)
    } finally {
      setIsOpeningAllFiles(false)
    }
  }, [openAllFilesDialog, openFilePathsInTabs])

  const confirmDelete = useCallback(async () => {
    if (!deleteCandidate) return
    setIsDeletingPath(true)
    try {
      await Promise.resolve(onDeletePath?.(deleteCandidate.path, deleteCandidate.isDir))
      setDeleteCandidate(null)
    } finally {
      setIsDeletingPath(false)
    }
  }, [deleteCandidate, onDeletePath])

  const dialogProps: Parameters<typeof FileExplorerDialogs>[0] = {
    alertDialog,
    onCloseAlertDialog: () => setAlertDialog(null),
    openAllFilesDialog,
    isOpeningAllFiles,
    onCloseOpenAllFilesDialog: () => setOpenAllFilesDialog(null),
    onConfirmOpenAllFiles: () => void confirmOpenAllFiles(),
    deleteCandidate,
    isDeletingPath,
    onCloseDeleteDialog: () => setDeleteCandidate(null),
    onConfirmDelete: () => void confirmDelete(),
    getPathBaseName,
  }

  return {
    showAlertDialog,
    requestDelete: setDeleteCandidate,
    openAllFilesInDirectory,
    dialogProps,
  }
}
