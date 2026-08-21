// Stub: file-system feature is out of scope for this session.
import { create } from 'zustand'
import { createSelectors } from '@/utils/zustand-selectors'
import type { AppFile } from '@/features/file-system/types/app'

// Task 28: addFolderToWorkspace/removeFolderFromWorkspace below are stub no-ops
// — Crowbar has no multi-root workspace model (daemon or FE) yet. While this is
// false, the file explorer hides its "Add Folder to Workspace" / "Remove From
// Workspace" context-menu items so no dead actions render; flip it when real
// implementations replace the stubs.
export const workspaceFoldersSupported: boolean = false

export interface FileSystemState {
  rootFolderPath: string | null
  handleOpenFolder: () => void
  handleOpenFolderByPath: ((path: string) => Promise<void>) | null
  handleFileOpen: ((path: string, revealOrIsDir?: boolean) => Promise<void>) | null
  handleFileSelect:
    | ((
        path: string,
        isDir?: boolean,
        line?: number,
        column?: number,
        extra?: unknown,
        reveal?: boolean,
      ) => void)
    | null
  addFolderToWorkspace: (path?: string) => Promise<void>
  removeFolderFromWorkspace: (path: string) => Promise<void>
  revealPathInTree: (path: string) => void | Promise<void>
  fileTree: AppFile[]
  files: AppFile[]
  setFiles: (files: AppFile[]) => void
  workspaceFolders: string[]
  isLoading: boolean
  isFileTreeLoading: boolean
  isSwitchingProject: boolean
  handleCreateNewFileInDirectory:
    ((dirPath: string, fileName?: string) => Promise<string | undefined>) | null
  handleCreateNewFolderInDirectory: ((dirPath: string, folderName?: string) => Promise<void>) | null
  handleDeletePath: ((path: string, isDir?: boolean) => Promise<void>) | null
  handleRevealInFolder: ((path: string) => void) | null
  handleFileMove: ((oldPath: string, newPath: string) => Promise<void>) | null
  handleDuplicatePath: ((path: string) => Promise<void>) | null
  handleRenamePath: ((path: string, newName?: string) => Promise<void>) | null
  refreshDirectory: ((path?: string) => Promise<void>) | null
}

const useFileSystemStoreBase = create<FileSystemState>((set) => ({
  rootFolderPath: null,
  handleOpenFolder: () => {},
  handleOpenFolderByPath: null,
  handleFileOpen: null,
  handleFileSelect: null,
  addFolderToWorkspace: async () => {},
  removeFolderFromWorkspace: async () => {},
  revealPathInTree: () => {},
  fileTree: [],
  files: [],
  setFiles: (files) => set({ fileTree: files, files }),
  workspaceFolders: [],
  isLoading: false,
  isFileTreeLoading: false,
  isSwitchingProject: false,
  handleCreateNewFileInDirectory: null,
  handleCreateNewFolderInDirectory: null,
  handleDeletePath: null,
  handleRevealInFolder: null,
  handleFileMove: null,
  handleDuplicatePath: null,
  handleRenamePath: null,
  refreshDirectory: null,
}))

export const useFileSystemStore = createSelectors(useFileSystemStoreBase)
