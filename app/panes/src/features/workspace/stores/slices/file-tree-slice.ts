import type { StateCreator } from 'zustand'
import type { AppFile } from '@/features/file-system/types/app'
import type { WorkspaceState } from '../workspace-store.types'

type FileTreeStatus = 'idle' | 'loading' | 'ready' | 'failed'

export interface FileTreeSlice {
  files: AppFile[]
  fileTreeStatus: FileTreeStatus
  fileTreeActions: {
    setFiles(files: AppFile[]): void
    setFileTreeStatus(status: FileTreeStatus): void
  }
}

export function isFileTreeLoading(state: Pick<FileTreeSlice, 'fileTreeStatus'>): boolean {
  return state.fileTreeStatus === 'idle' || state.fileTreeStatus === 'loading'
}

export const createFileTreeSlice: StateCreator<
  WorkspaceState,
  [['zustand/immer', never]],
  [],
  FileTreeSlice
> = (set) => ({
  files: [],
  fileTreeStatus: 'idle',
  fileTreeActions: {
    setFiles: (files) => set({ files }),
    setFileTreeStatus: (fileTreeStatus) => set({ fileTreeStatus }),
  },
})
