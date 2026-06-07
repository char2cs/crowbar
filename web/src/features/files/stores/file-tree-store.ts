import { create } from 'zustand'
import { createLoadableSlice, type LoadableSlice } from '@/lib/store/loadable-slice'
import { apiFetch } from '@/lib/api'
import type { FileNode } from '@/lib/mock/files'

export const useFileTreeStore = create<LoadableSlice<FileNode>>()((set, get) =>
  createLoadableSlice<FileNode>({
    store: 'file-tree-data',
    fetcher: (rootPath: string) =>
      apiFetch<FileNode>(`/v0/fs/tree?root=${encodeURIComponent(rootPath)}`),
    wsEndpoint: (wsId: string) =>
      `/v0/ws/files?wsId=${encodeURIComponent(wsId)}`,
  })(set, get),
)
