import { create } from 'zustand'
import { createLoadableSlice, type LoadableSlice } from '@/lib/store/loadable-slice'
import { apiFetch } from '@/lib/api'
import type { FileNode } from '@/lib/mock/files'

export const useFileTreeStore = create<LoadableSlice<FileNode>>()((set, get) =>
  createLoadableSlice<FileNode>({
    store: 'file-tree-data',
    fetcher: (rootPath: string) =>
      apiFetch<FileNode>(`/v0/fs/tree?root=${encodeURIComponent(rootPath)}`),
    wsEndpoint: (rootPath: string) =>
      `/v0/ws/files?workspaceId=${encodeURIComponent(rootPath)}`,
  })(set, get),
)
